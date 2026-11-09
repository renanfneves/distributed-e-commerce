package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/services"

	"github.com/google/uuid"
)

func TestReleaseInventoryReservationSuccess(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(ctx, "INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, $2)", skuID, 10); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}
	orderID := uuid.New()
	reservationID := uuid.New()
	createdReservation, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		ReservationID: reservationID,
		OrderID:       orderID,
		SkuID:         skuID,
		Quantity:      4,
	})
	if err != nil {
		t.Fatalf("preparar estoque em CreateInventoryReservation: %v", err)
	}
	if !createdReservation.UpdatedAt.Valid {
		t.Fatal("updated_at inicial inválido; esperado timestamp válido")
	}
	// Act
	reservation, err := service.ReleaseInventoryReservation(ctx, reservationID)

	// Assert
	if err != nil {
		t.Fatalf("preparar estoque em ReleaseInventoryReservation: %v", err)
	}
	if reservationID != reservation.ReservationID {
		t.Fatalf("reservationId enviado: %v não é o mesmo retornado %v", reservationID, reservation.ReservationID)
	}
	if reservation.Status != database.InventoryReservationStatusReleased {
		t.Fatalf("status retornado: %v não compatível com o desejado %v", reservation.Status, database.InventoryReservationStatusReleased)
	}
	if !reservation.UpdatedAt.Valid {
		t.Fatal("updated_at da reserva liberada inválido; esperado timestamp válido")
	}
	if !reservation.UpdatedAt.Time.After(
		createdReservation.UpdatedAt.Time,
	) {
		t.Errorf(
			"updated_at após liberação = %v; esperado valor posterior a %v",
			reservation.UpdatedAt.Time,
			createdReservation.UpdatedAt.Time,
		)
	}
	var reservationDatabaseStatus database.InventoryReservationStatus
	var persistedUpdatedAt time.Time
	err = pool.QueryRow(
		ctx,
		`
		SELECT status, updated_at
		FROM inventory_reservations
		WHERE reservation_id = $1
	`,
		reservationID,
	).Scan(&reservationDatabaseStatus, &persistedUpdatedAt)
	if err != nil {
		t.Fatalf("consultar existência da reserva: %v", err)
	}
	if reservationDatabaseStatus != database.InventoryReservationStatusReleased {
		t.Fatalf("status da reserva persistida deveria ser: %v e retornou: %v", database.InventoryReservationStatusReleased, reservationDatabaseStatus)
	}
	if !persistedUpdatedAt.Equal(reservation.UpdatedAt.Time) {
		t.Errorf(
			"updated_at persistido = %v; esperado %v",
			persistedUpdatedAt,
			reservation.UpdatedAt.Time,
		)
	}
	assertInventoryState(t, pool, ctx, skuID, 10, 0, 10)
}

func TestReleaseInventoryReservationIdempotentRetry(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, $2)",
		skuID,
		10,
	); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	reservationID := uuid.New()
	if _, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		ReservationID: reservationID,
		OrderID:       uuid.New(),
		SkuID:         skuID,
		Quantity:      4,
	}); err != nil {
		t.Fatalf("preparar reserva: %v", err)
	}

	// Act
	firstRelease, firstErr := service.ReleaseInventoryReservation(ctx, reservationID)
	secondRelease, secondErr := service.ReleaseInventoryReservation(ctx, reservationID)

	// Assert
	if firstErr != nil {
		t.Fatalf("primeira ReleaseInventoryReservation() erro = %v; esperado nil", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("retry de ReleaseInventoryReservation() erro = %v; esperado nil", secondErr)
	}
	if firstRelease.ReservationID != reservationID {
		t.Errorf(
			"primeira liberação reservation_id = %v; esperado %v",
			firstRelease.ReservationID,
			reservationID,
		)
	}
	if secondRelease.ReservationID != reservationID {
		t.Errorf(
			"retry reservation_id = %v; esperado %v",
			secondRelease.ReservationID,
			reservationID,
		)
	}
	if firstRelease.Status != database.InventoryReservationStatusReleased {
		t.Errorf(
			"status da primeira liberação = %v; esperado %v",
			firstRelease.Status,
			database.InventoryReservationStatusReleased,
		)
	}
	if secondRelease.Status != database.InventoryReservationStatusReleased {
		t.Errorf(
			"status do retry = %v; esperado %v",
			secondRelease.Status,
			database.InventoryReservationStatusReleased,
		)
	}
	if !firstRelease.UpdatedAt.Valid {
		t.Fatal("updated_at da primeira liberação inválido; esperado timestamp válido")
	}
	if !secondRelease.UpdatedAt.Valid {
		t.Fatal("updated_at do retry inválido; esperado timestamp válido")
	}
	if !secondRelease.UpdatedAt.Time.Equal(firstRelease.UpdatedAt.Time) {
		t.Errorf(
			"updated_at do retry = %v; esperado permanecer %v",
			secondRelease.UpdatedAt.Time,
			firstRelease.UpdatedAt.Time,
		)
	}

	var persistedStatus database.InventoryReservationStatus
	var persistedUpdatedAt time.Time
	if err := pool.QueryRow(
		ctx,
		`
			SELECT status, updated_at
			FROM inventory_reservations
			WHERE reservation_id = $1
		`,
		reservationID,
	).Scan(&persistedStatus, &persistedUpdatedAt); err != nil {
		t.Fatalf("consultar reserva após retry: %v", err)
	}
	if persistedStatus != database.InventoryReservationStatusReleased {
		t.Errorf(
			"status persistido após retry = %v; esperado %v",
			persistedStatus,
			database.InventoryReservationStatusReleased,
		)
	}
	if !persistedUpdatedAt.Equal(firstRelease.UpdatedAt.Time) {
		t.Errorf(
			"updated_at persistido após retry = %v; esperado %v",
			persistedUpdatedAt,
			firstRelease.UpdatedAt.Time,
		)
	}

	assertInventoryState(t, pool, ctx, skuID, 10, 0, 10)
}

func TestReleaseInventoryReservationInvalidInput(t *testing.T) {
	// Arrange
	service := services.NewInventoryService(nil, testReservationTTL)

	// Act
	reservation, err := service.ReleaseInventoryReservation(context.Background(), uuid.Nil)

	// Assert
	if !errors.Is(err, services.ErrInvalidReservationInput) {
		t.Errorf(
			"ReleaseInventoryReservation() erro = %v; esperado %v",
			err,
			services.ErrInvalidReservationInput,
		)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf(
			"ReleaseInventoryReservation() reserva = %+v; esperado valor zero",
			reservation,
		)
	}
}

func TestReleaseInventoryReservationNotFound(t *testing.T) {
	// Arrange
	service, _, ctx := setupIntegrationService(t)
	missingReservationID := uuid.New()

	// Act
	reservation, err := service.ReleaseInventoryReservation(ctx, missingReservationID)

	// Assert
	if !errors.Is(err, services.ErrInventoryReservationNotFound) {
		t.Errorf(
			"ReleaseInventoryReservation() erro = %v; esperado %v",
			err,
			services.ErrInventoryReservationNotFound,
		)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf(
			"ReleaseInventoryReservation() reserva = %+v; esperado valor zero",
			reservation,
		)
	}
}

func TestReleaseInventoryReservationConfirmedConflict(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, $2)",
		skuID,
		10,
	); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	reservationID := uuid.New()
	if _, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		ReservationID: reservationID,
		OrderID:       uuid.New(),
		SkuID:         skuID,
		Quantity:      4,
	}); err != nil {
		t.Fatalf("preparar reserva: %v", err)
	}

	var confirmedUpdatedAt time.Time
	if err := pool.QueryRow(
		ctx,
		`
			UPDATE inventory_reservations
			SET status = 'confirmed', updated_at = NOW()
			WHERE reservation_id = $1
			RETURNING updated_at
		`,
		reservationID,
	).Scan(&confirmedUpdatedAt); err != nil {
		t.Fatalf("preparar reserva confirmada: %v", err)
	}

	// Act
	reservation, err := service.ReleaseInventoryReservation(ctx, reservationID)

	// Assert
	if !errors.Is(err, services.ErrInventoryReservationTransitionConflict) {
		t.Errorf(
			"ReleaseInventoryReservation() erro = %v; esperado %v",
			err,
			services.ErrInventoryReservationTransitionConflict,
		)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf(
			"ReleaseInventoryReservation() reserva = %+v; esperado valor zero",
			reservation,
		)
	}

	var persistedStatus database.InventoryReservationStatus
	var persistedUpdatedAt time.Time
	if err := pool.QueryRow(
		ctx,
		`
			SELECT status, updated_at
			FROM inventory_reservations
			WHERE reservation_id = $1
		`,
		reservationID,
	).Scan(&persistedStatus, &persistedUpdatedAt); err != nil {
		t.Fatalf("consultar reserva após conflito: %v", err)
	}
	if persistedStatus != database.InventoryReservationStatusConfirmed {
		t.Errorf(
			"status persistido após conflito = %v; esperado %v",
			persistedStatus,
			database.InventoryReservationStatusConfirmed,
		)
	}
	if !persistedUpdatedAt.Equal(confirmedUpdatedAt) {
		t.Errorf(
			"updated_at após conflito = %v; esperado permanecer %v",
			persistedUpdatedAt,
			confirmedUpdatedAt,
		)
	}
	assertInventoryState(t, pool, ctx, skuID, 10, 4, 6)
}

func TestReleaseInventoryReservationConcurrent(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, $2)",
		skuID,
		10,
	); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	reservationID := uuid.New()
	if _, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		ReservationID: reservationID,
		OrderID:       uuid.New(),
		SkuID:         skuID,
		Quantity:      4,
	}); err != nil {
		t.Fatalf("preparar reserva: %v", err)
	}

	type releaseResult struct {
		reservation database.InventoryReservation
		err         error
	}

	const attempts = 2
	start := make(chan struct{})
	results := make(chan releaseResult, attempts)
	for range attempts {
		go func() {
			<-start
			reservation, err := service.ReleaseInventoryReservation(ctx, reservationID)
			results <- releaseResult{reservation: reservation, err: err}
		}()
	}

	// Act
	close(start)

	// Assert
	releases := make([]database.InventoryReservation, 0, attempts)
	for range attempts {
		select {
		case result := <-results:
			if result.err != nil {
				t.Errorf(
					"ReleaseInventoryReservation() concorrente erro = %v; esperado nil",
					result.err,
				)
				continue
			}
			releases = append(releases, result.reservation)
		case <-ctx.Done():
			t.Fatalf("tempo esgotado aguardando liberações concorrentes: %v", ctx.Err())
		}
	}

	if len(releases) != attempts {
		t.Fatalf(
			"liberações concorrentes bem-sucedidas = %d; esperado %d",
			len(releases),
			attempts,
		)
	}
	for index, reservation := range releases {
		if reservation.ReservationID != reservationID {
			t.Errorf(
				"liberação concorrente %d reservation_id = %v; esperado %v",
				index+1,
				reservation.ReservationID,
				reservationID,
			)
		}
		if reservation.Status != database.InventoryReservationStatusReleased {
			t.Errorf(
				"liberação concorrente %d status = %v; esperado %v",
				index+1,
				reservation.Status,
				database.InventoryReservationStatusReleased,
			)
		}
		if !reservation.UpdatedAt.Valid {
			t.Fatalf(
				"liberação concorrente %d updated_at inválido; esperado timestamp válido",
				index+1,
			)
		}
	}
	if !releases[0].UpdatedAt.Time.Equal(releases[1].UpdatedAt.Time) {
		t.Errorf(
			"updated_at das liberações concorrentes = (%v, %v); esperado mesmo valor",
			releases[0].UpdatedAt.Time,
			releases[1].UpdatedAt.Time,
		)
	}

	var persistedStatus database.InventoryReservationStatus
	var persistedUpdatedAt time.Time
	if err := pool.QueryRow(
		ctx,
		`
			SELECT status, updated_at
			FROM inventory_reservations
			WHERE reservation_id = $1
		`,
		reservationID,
	).Scan(&persistedStatus, &persistedUpdatedAt); err != nil {
		t.Fatalf("consultar reserva após liberações concorrentes: %v", err)
	}
	if persistedStatus != database.InventoryReservationStatusReleased {
		t.Errorf(
			"status persistido após liberações concorrentes = %v; esperado %v",
			persistedStatus,
			database.InventoryReservationStatusReleased,
		)
	}
	if !persistedUpdatedAt.Equal(releases[0].UpdatedAt.Time) {
		t.Errorf(
			"updated_at persistido após liberações concorrentes = %v; esperado %v",
			persistedUpdatedAt,
			releases[0].UpdatedAt.Time,
		)
	}
	assertInventoryState(t, pool, ctx, skuID, 10, 0, 10)
}
