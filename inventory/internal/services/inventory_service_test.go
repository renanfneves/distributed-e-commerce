package services_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/services"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	uri := os.Getenv("TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("defina TEST_DATABASE_URL para executar os testes de integração")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		t.Fatalf("criar pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("conectar ao banco de testes: %v", err)
	}
	return pool
}

func cleanupInventory(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM inventory_reservations WHERE sku_id = $1", id); err != nil {
			t.Fatalf("limpar reservas de teste: %v", err)
			return
		}
		if _, err := pool.Exec(ctx, "DELETE FROM inventory_items WHERE sku_id = $1", id); err != nil {
			t.Errorf("limpar item de teste: %v", err)
		}
	})
}

func setupIntegrationService(t *testing.T) (services.InventoryService, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := integrationPool(t)
	service := services.NewInventoryService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return service, pool, ctx
}

func newTestSKU(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	skuID := uuid.New()
	cleanupInventory(t, pool, skuID)

	return skuID
}

func assertInventoryState(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	skuID uuid.UUID,
	wantOnHand int64,
	wantReserved int64,
	wantAvailable int64,
) {
	t.Helper()

	var gotOnHand, gotReserved, gotAvailable int64
	if err := pool.QueryRow(
		ctx,
		`SELECT
			quantity_on_hand,
			quantity_reserved,
			quantity_on_hand - quantity_reserved
		FROM inventory_items
		WHERE sku_id = $1`,
		skuID,
	).Scan(&gotOnHand, &gotReserved, &gotAvailable); err != nil {
		t.Fatalf("consultar estoque do SKU %s: %v", skuID, err)
	}

	if gotOnHand != wantOnHand ||
		gotReserved != wantReserved ||
		gotAvailable != wantAvailable {
		t.Errorf(
			"estado do estoque = (on_hand=%d, reserved=%d, available=%d); esperado (%d, %d, %d)",
			gotOnHand,
			gotReserved,
			gotAvailable,
			wantOnHand,
			wantReserved,
			wantAvailable,
		)
	}
}

func TestCreateInventoryItemIntegrationSuccess(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewInventoryService(pool)
	for _, quantity := range []int64{0, 10, 1 << 33} {
		t.Run(fmt.Sprintf("quantity_%d", quantity), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			skuID := uuid.New()
			cleanupInventory(t, pool, skuID)
			id, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: quantity})
			if err != nil {
				t.Fatalf("CreateInventoryItem() erro = %v; esperado nil", err)
			}
			if id != skuID {
				t.Errorf("CreateInventoryItem() id = %s; esperado %s", id, skuID)
			}
			var onHand, reserved int64
			if err := pool.QueryRow(ctx, "SELECT quantity_on_hand, quantity_reserved FROM inventory_items WHERE sku_id = $1", skuID).Scan(&onHand, &reserved); err != nil {
				t.Fatalf("consultar persistência: %v", err)
			}
			if onHand != quantity || reserved != 0 {
				t.Errorf(
					"quantidades persistidas = (on_hand=%d, reserved=%d); esperado (%d, 0)",
					onHand,
					reserved,
					quantity,
				)
			}
			item, err := service.GetInventoryItemBySKU(ctx, skuID)
			if err != nil {
				t.Fatalf("GetInventoryItemBySKU() erro = %v; esperado nil", err)
			}
			if item.SkuID != skuID || item.QuantityOnHand != quantity || item.QuantityReserved != 0 || item.QuantityAvailable != quantity {
				t.Errorf(
					"GetInventoryItemBySKU() item = %+v; esperado sku_id=%s on_hand=%d reserved=0 available=%d",
					item,
					skuID,
					quantity,
					quantity,
				)
			}
		})
	}
}

func TestCreateInventoryItemIntegrationDuplicate(t *testing.T) {
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)

	if _, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: 10}); err != nil {
		t.Fatalf("preparar item original: %v", err)
	}
	id, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: 99})
	if !errors.Is(err, services.ErrInventoryItemAlreadyExists) {
		t.Errorf("CreateInventoryItem() erro = %v; esperado %v", err, services.ErrInventoryItemAlreadyExists)
	}
	if id != uuid.Nil {
		t.Errorf("CreateInventoryItem() id = %s; esperado UUID vazio", id)
	}
	var quantity int64
	if err := pool.QueryRow(ctx, "SELECT quantity_on_hand FROM inventory_items WHERE sku_id = $1", skuID).Scan(&quantity); err != nil {
		t.Fatalf("verificar original: %v", err)
	}
	if quantity != 10 {
		t.Errorf("quantidade original = %d; esperado 10", quantity)
	}
}

func TestCreateInventoryItemInvalidQuantity(t *testing.T) {
	// Sem queries: a validação deve rejeitar a entrada antes de acessar o banco.
	service := services.NewInventoryService(nil)
	id, err := service.CreateInventoryItem(context.Background(), services.InventoryItemInput{SkuID: uuid.New(), QuantityOnHand: -1})
	if !errors.Is(err, services.ErrInvalidQuantity) {
		t.Errorf("CreateInventoryItem() erro = %v; esperado %v", err, services.ErrInvalidQuantity)
	}
	if id != uuid.Nil {
		t.Errorf("CreateInventoryItem() id = %s; esperado UUID vazio", id)
	}
}

func TestGetInventoryItemIntegrationNotFound(t *testing.T) {
	service, _, ctx := setupIntegrationService(t)

	item, err := service.GetInventoryItemBySKU(ctx, uuid.New())
	if !errors.Is(err, services.ErrInventoryItemNotFound) {
		t.Errorf("GetInventoryItemBySKU() erro = %v; esperado %v", err, services.ErrInventoryItemNotFound)
	}
	if item != (database.GetInventoryItemBySKURow{}) {
		t.Errorf("GetInventoryItemBySKU() item = %+v; esperado valor zero", item)
	}
}

func TestGetInventoryItemIntegrationAvailability(t *testing.T) {
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)

	if _, err := pool.Exec(ctx, "INSERT INTO inventory_items (sku_id, quantity_on_hand, quantity_reserved) VALUES ($1, 10, 4)", skuID); err != nil {
		t.Fatalf("preparar estoque reservado: %v", err)
	}
	item, err := service.GetInventoryItemBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("GetInventoryItemBySKU() erro = %v; esperado nil", err)
	}
	if item.SkuID != skuID || item.QuantityOnHand != 10 || item.QuantityReserved != 4 || item.QuantityAvailable != 6 {
		t.Errorf(
			"GetInventoryItemBySKU() item = %+v; esperado sku_id=%s on_hand=10 reserved=4 available=6",
			item,
			skuID,
		)
	}
}

func TestReserveInventoryIntegrationConcurrent(t *testing.T) {
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)

	_, err := service.CreateInventoryItem(
		ctx,
		services.InventoryItemInput{
			SkuID:          skuID,
			QuantityOnHand: 10,
		},
	)
	if err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)

	for range 2 {
		go func() {
			<-start
			_, err := service.ReserveInventory(ctx, skuID, 7)
			results <- err
		}()
	}

	close(start)

	successes := 0
	rejections := 0

	for range 2 {
		select {
		case err := <-results:
			switch {
			case err == nil:
				successes++

			case errors.Is(
				err,
				services.ErrInventoryReservationRejected,
			):
				rejections++

			default:
				t.Errorf("ReserveInventory() erro = %v; esperado nil ou %v", err, services.ErrInventoryReservationRejected)
			}

		case <-ctx.Done():
			t.Fatalf(
				"tempo esgotado aguardando as reservas: %v",
				ctx.Err(),
			)
		}
	}

	if successes != 1 || rejections != 1 {
		t.Errorf(
			"resultado concorrente = (sucessos=%d, rejeições=%d); esperado (1, 1)",
			successes,
			rejections,
		)
	}

	assertInventoryState(t, pool, ctx, skuID, 10, 7, 3)
}

func TestCreateInventoryReservationIntegrationSuccess(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(ctx, "INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, $2)", skuID, 10); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}
	orderID := uuid.New()
	reservationID := uuid.New()

	// Act
	reservation, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		SkuID:         skuID,
		OrderID:       orderID,
		ReservationID: reservationID,
		Quantity:      4,
	})

	// Assert
	if err != nil {
		t.Fatalf("CreateInventoryReservation() erro = %v; esperado nil", err)
	}
	var reservationExists bool

	err = pool.QueryRow(
		ctx,
		`SELECT EXISTS (
		SELECT 1
		FROM inventory_reservations
		WHERE reservation_id = $1
	)`,
		reservationID,
	).Scan(&reservationExists)
	if err != nil {
		t.Fatalf("consultar existência da reserva: %v", err)
	}

	if !reservationExists {
		t.Errorf(
			"reserva persistida = false para reservation_id=%s; esperado true",
			reservationID,
		)
	}
	if reservation.SkuID != skuID {
		t.Errorf(
			"reservation.SkuID = %s; esperado %s",
			reservation.SkuID,
			skuID,
		)
	}
	if reservation.OrderID != orderID {
		t.Errorf(
			"reservation.OrderID = %s; esperado %s",
			reservation.OrderID,
			orderID,
		)
	}
	if reservation.ReservationID != reservationID {
		t.Errorf(
			"reservation.ReservationID = %s; esperado %s",
			reservation.ReservationID,
			reservationID,
		)
	}
	if reservation.Quantity != 4 {
		t.Errorf(
			"reservation.Quantity = %d; esperado %d",
			reservation.Quantity,
			4,
		)
	}
	if reservation.Status != database.InventoryReservationStatusActive {
		t.Errorf(
			"reservation.Status = %s; esperado %s",
			reservation.Status,
			database.InventoryReservationStatusActive,
		)
	}
	assertInventoryState(t, pool, ctx, skuID, 10, 4, 6)
}

func TestCreateInventoryReservationIntegrationInsufficientStockRollback(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	reservationID := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, $2)", skuID, 3); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	// Act
	reservation, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		SkuID:         skuID,
		OrderID:       uuid.New(),
		ReservationID: reservationID,
		Quantity:      5,
	})

	// Assert

	if !errors.Is(err, services.ErrInventoryReservationRejected) {
		t.Errorf(
			"CreateInventoryReservation() erro = %v; esperado %v",
			err,
			services.ErrInventoryReservationRejected,
		)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf(
			"CreateInventoryReservation() reserva = %+v; esperado valor zero",
			reservation,
		)
	}
	var reservationExists bool
	err = pool.QueryRow(
		ctx,
		`SELECT EXISTS (
		SELECT 1
		FROM inventory_reservations
		WHERE reservation_id = $1
	)`,
		reservationID,
	).Scan(&reservationExists)
	if err != nil {
		t.Fatalf(
			"consultar existência da reserva: %v",
			err,
		)
	}
	if reservationExists {
		t.Errorf(
			"reserva persistida = true para reservation_id=%s; esperado false após rollback",
			reservationID,
		)
	}

	assertInventoryState(t, pool, ctx, skuID, 3, 0, 3)
}

func TestCreateInventoryReservationIntegrationIdempotentRetry(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	orderID := uuid.New()
	reservationID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO
			inventory_items (sku_id, quantity_on_hand)
		VALUES
			($1, $2)`, skuID, 10); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}
	input := services.InventoryReservationInput{
		SkuID:         skuID,
		OrderID:       orderID,
		ReservationID: reservationID,
		Quantity:      4,
	}

	// Act
	resultOne, errOne := service.CreateInventoryReservation(ctx, input)
	if errOne != nil {
		t.Fatalf("primeira CreateInventoryReservation() erro = %v; esperado nil", errOne)
	}
	resultTwo, errTwo := service.CreateInventoryReservation(ctx, input)
	if errTwo != nil {
		t.Fatalf("retry de CreateInventoryReservation() erro = %v; esperado nil", errTwo)
	}
	if resultOne != resultTwo {
		t.Errorf(
			"retry retornou reserva diferente: primeira=%+v retry=%+v",
			resultOne,
			resultTwo,
		)
	}

	assertInventoryState(t, pool, ctx, skuID, 10, 4, 6)

	var storedReservationCount int64
	err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
	FROM inventory_reservations
	WHERE reservation_id = $1`,
		reservationID,
	).Scan(&storedReservationCount)

	if err != nil {
		t.Fatalf("contar reservas persistidas: %v", err)
	}

	if storedReservationCount != 1 {
		t.Errorf(
			"quantidade de reservas persistidas = %d; esperado 1",
			storedReservationCount,
		)
	}
}

func TestCreateInventoryReservationInvalidInput(t *testing.T) {
	validInput := services.InventoryReservationInput{
		ReservationID: uuid.New(),
		OrderID:       uuid.New(),
		SkuID:         uuid.New(),
		Quantity:      1,
	}

	tests := []struct {
		name   string
		mutate func(*services.InventoryReservationInput)
	}{
		{
			name: "reservation_id vazio",
			mutate: func(input *services.InventoryReservationInput) {
				input.ReservationID = uuid.Nil
			},
		},
		{
			name: "order_id vazio",
			mutate: func(input *services.InventoryReservationInput) {
				input.OrderID = uuid.Nil
			},
		},
		{
			name: "sku_id vazio",
			mutate: func(input *services.InventoryReservationInput) {
				input.SkuID = uuid.Nil
			},
		},
		{
			name: "quantidade zero",
			mutate: func(input *services.InventoryReservationInput) {
				input.Quantity = 0
			},
		},
		{
			name: "quantidade negativa",
			mutate: func(input *services.InventoryReservationInput) {
				input.Quantity = -1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A validação deve falhar antes de acessar o pool nil.
			service := services.NewInventoryService(nil)
			input := validInput
			test.mutate(&input)

			reservation, err := service.CreateInventoryReservation(
				context.Background(),
				input,
			)

			if !errors.Is(err, services.ErrInvalidReservationInput) {
				t.Errorf(
					"CreateInventoryReservation() erro = %v; esperado %v",
					err,
					services.ErrInvalidReservationInput,
				)
			}
			if reservation != (database.InventoryReservation{}) {
				t.Errorf(
					"CreateInventoryReservation() reserva = %+v; esperado valor zero",
					reservation,
				)
			}
		})
	}
}

func TestCreateInventoryReservationIntegrationMissingSKU(t *testing.T) {
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	reservationID := uuid.New()

	reservation, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		ReservationID: reservationID,
		OrderID:       uuid.New(),
		SkuID:         skuID,
		Quantity:      1,
	})

	if !errors.Is(err, services.ErrInventoryReservationRejected) {
		t.Errorf(
			"CreateInventoryReservation() erro = %v; esperado %v",
			err,
			services.ErrInventoryReservationRejected,
		)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf(
			"CreateInventoryReservation() reserva = %+v; esperado valor zero",
			reservation,
		)
	}

	var storedReservationCount int64
	if err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM inventory_reservations WHERE reservation_id = $1",
		reservationID,
	).Scan(&storedReservationCount); err != nil {
		t.Fatalf("contar reservas persistidas: %v", err)
	}
	if storedReservationCount != 0 {
		t.Errorf(
			"quantidade de reservas persistidas = %d; esperado 0",
			storedReservationCount,
		)
	}
}

func TestCreateInventoryReservationIntegrationIdempotencyConflict(t *testing.T) {
	tests := []struct {
		name          string
		conflictInput func(services.InventoryReservationInput, uuid.UUID) services.InventoryReservationInput
	}{
		{
			name: "order_id diferente",
			conflictInput: func(input services.InventoryReservationInput, _ uuid.UUID) services.InventoryReservationInput {
				input.OrderID = uuid.New()
				return input
			},
		},
		{
			name: "sku_id diferente",
			conflictInput: func(input services.InventoryReservationInput, alternateSKU uuid.UUID) services.InventoryReservationInput {
				input.SkuID = alternateSKU
				return input
			},
		},
		{
			name: "quantidade diferente",
			conflictInput: func(input services.InventoryReservationInput, _ uuid.UUID) services.InventoryReservationInput {
				input.Quantity = 5
				return input
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, pool, ctx := setupIntegrationService(t)
			originalSKU := newTestSKU(t, pool)
			alternateSKU := newTestSKU(t, pool)
			if _, err := pool.Exec(
				ctx,
				`INSERT INTO inventory_items (sku_id, quantity_on_hand)
				VALUES ($1, 10), ($2, 10)`,
				originalSKU,
				alternateSKU,
			); err != nil {
				t.Fatalf("preparar estoques: %v", err)
			}

			originalInput := services.InventoryReservationInput{
				ReservationID: uuid.New(),
				OrderID:       uuid.New(),
				SkuID:         originalSKU,
				Quantity:      4,
			}
			originalReservation, err := service.CreateInventoryReservation(ctx, originalInput)
			if err != nil {
				t.Fatalf("primeira CreateInventoryReservation() erro = %v; esperado nil", err)
			}

			conflictingInput := test.conflictInput(originalInput, alternateSKU)
			conflictingReservation, err := service.CreateInventoryReservation(ctx, conflictingInput)

			if !errors.Is(err, services.ErrReservationIdempotencyConflict) {
				t.Errorf(
					"CreateInventoryReservation() erro = %v; esperado %v",
					err,
					services.ErrReservationIdempotencyConflict,
				)
			}
			if conflictingReservation != (database.InventoryReservation{}) {
				t.Errorf(
					"CreateInventoryReservation() reserva = %+v; esperado valor zero",
					conflictingReservation,
				)
			}

			storedReservation, err := database.New(pool).GetInventoryReservationByID(
				ctx,
				originalInput.ReservationID,
			)
			if err != nil {
				t.Fatalf("consultar reserva original: %v", err)
			}
			if storedReservation != originalReservation {
				t.Errorf(
					"reserva persistida = %+v; esperado %+v",
					storedReservation,
					originalReservation,
				)
			}

			var storedReservationCount int64
			if err := pool.QueryRow(
				ctx,
				"SELECT COUNT(*) FROM inventory_reservations WHERE reservation_id = $1",
				originalInput.ReservationID,
			).Scan(&storedReservationCount); err != nil {
				t.Fatalf("contar reservas persistidas: %v", err)
			}
			if storedReservationCount != 1 {
				t.Errorf(
					"quantidade de reservas persistidas = %d; esperado 1",
					storedReservationCount,
				)
			}

			assertInventoryState(t, pool, ctx, originalSKU, 10, 4, 6)
			assertInventoryState(t, pool, ctx, alternateSKU, 10, 0, 10)
		})
	}
}

func TestCreateInventoryReservationIntegrationConcurrent(t *testing.T) {
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, 10)",
		skuID,
	); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	inputs := []services.InventoryReservationInput{
		{
			ReservationID: uuid.New(),
			OrderID:       uuid.New(),
			SkuID:         skuID,
			Quantity:      7,
		},
		{
			ReservationID: uuid.New(),
			OrderID:       uuid.New(),
			SkuID:         skuID,
			Quantity:      7,
		},
	}

	type attemptResult struct {
		input       services.InventoryReservationInput
		reservation database.InventoryReservation
		err         error
	}

	start := make(chan struct{})
	results := make(chan attemptResult, len(inputs))
	for _, input := range inputs {
		input := input
		go func() {
			<-start
			reservation, err := service.CreateInventoryReservation(ctx, input)
			results <- attemptResult{input: input, reservation: reservation, err: err}
		}()
	}
	close(start)

	var successful, rejected *attemptResult
	for range inputs {
		select {
		case result := <-results:
			switch {
			case result.err == nil:
				result := result
				successful = &result
			case errors.Is(result.err, services.ErrInventoryReservationRejected):
				result := result
				rejected = &result
			default:
				t.Errorf(
					"CreateInventoryReservation() erro = %v; esperado nil ou %v",
					result.err,
					services.ErrInventoryReservationRejected,
				)
			}
		case <-ctx.Done():
			t.Fatalf("tempo esgotado aguardando reservas concorrentes: %v", ctx.Err())
		}
	}

	if successful == nil || rejected == nil {
		t.Fatalf(
			"resultado concorrente = (sucesso=%t, rejeição=%t); esperado (true, true)",
			successful != nil,
			rejected != nil,
		)
	}
	if successful.reservation.ReservationID != successful.input.ReservationID {
		t.Errorf(
			"reserva bem-sucedida reservation_id = %s; esperado %s",
			successful.reservation.ReservationID,
			successful.input.ReservationID,
		)
	}
	if rejected.reservation != (database.InventoryReservation{}) {
		t.Errorf(
			"reserva rejeitada = %+v; esperado valor zero",
			rejected.reservation,
		)
	}

	var storedReservationCount int64
	if err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM inventory_reservations WHERE sku_id = $1",
		skuID,
	).Scan(&storedReservationCount); err != nil {
		t.Fatalf("contar reservas persistidas: %v", err)
	}
	if storedReservationCount != 1 {
		t.Errorf(
			"quantidade de reservas persistidas = %d; esperado 1",
			storedReservationCount,
		)
	}

	var rejectedReservationCount int64
	if err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM inventory_reservations WHERE reservation_id = $1",
		rejected.input.ReservationID,
	).Scan(&rejectedReservationCount); err != nil {
		t.Fatalf("contar reserva rejeitada: %v", err)
	}
	if rejectedReservationCount != 0 {
		t.Errorf(
			"quantidade de reservas rejeitadas persistidas = %d; esperado 0",
			rejectedReservationCount,
		)
	}

	assertInventoryState(t, pool, ctx, skuID, 10, 7, 3)
}
