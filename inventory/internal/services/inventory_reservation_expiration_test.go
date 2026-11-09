package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/services"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestProcessExpiredReservationsInvalidBatchSize(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int32
	}{
		{name: "zero", batchSize: 0},
		{name: "negativo", batchSize: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			service := services.NewInventoryService(nil, testReservationTTL)

			// Act
			result, err := service.ProcessExpiredReservations(
				context.Background(),
				test.batchSize,
			)

			// Assert
			if !errors.Is(err, services.ErrInvalidExpirationBatchSize) {
				t.Errorf(
					"ProcessExpiredReservations() erro = %v; esperado %v",
					err,
					services.ErrInvalidExpirationBatchSize,
				)
			}
			if result != (services.ExpiredReservationsBatchResult{}) {
				t.Errorf(
					"ProcessExpiredReservations() resultado = %+v; esperado valor zero",
					result,
				)
			}
		})
	}
}

func TestProcessExpiredReservationsIntegrationEmptyBatch(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(t, service, pool, ctx, time.Now().Add(time.Hour), 4)

	// Act
	result, err := service.ProcessExpiredReservations(ctx, 10)

	// Assert
	if err != nil {
		t.Fatalf("ProcessExpiredReservations() erro = %v; esperado nil", err)
	}
	want := services.ExpiredReservationsBatchResult{}
	if result != want {
		t.Errorf("ProcessExpiredReservations() resultado = %+v; esperado %+v", result, want)
	}
	assertReservationStatus(t, pool, ctx, reservationID, database.InventoryReservationStatusActive)
	assertInventoryState(t, pool, ctx, skuID, 10, 4, 6)
}

func TestProcessExpiredReservationsIntegrationOnlyExpired(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	expiredSKU, expiredReservationID := prepareReservationWithExpiration(t, service, pool, ctx, time.Now().Add(-time.Hour), 4)
	activeSKU, activeReservationID := prepareReservationWithExpiration(t, service, pool, ctx, time.Now().Add(time.Hour), 3)

	// Act
	result, err := service.ProcessExpiredReservations(ctx, 10)

	// Assert
	if err != nil {
		t.Fatalf("ProcessExpiredReservations() erro = %v; esperado nil", err)
	}
	want := services.ExpiredReservationsBatchResult{
		Selected:  1,
		Processed: 1,
	}
	if result != want {
		t.Errorf("ProcessExpiredReservations() resultado = %+v; esperado %+v", result, want)
	}
	assertReservationStatus(t, pool, ctx, expiredReservationID, database.InventoryReservationStatusReleased)
	assertReservationStatus(t, pool, ctx, activeReservationID, database.InventoryReservationStatusActive)
	assertInventoryState(t, pool, ctx, expiredSKU, 10, 0, 10)
	assertInventoryState(t, pool, ctx, activeSKU, 10, 3, 7)
}

func TestProcessExpiredReservationsIntegrationRespectsBatchOrder(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	baseExpiration := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	oldestSKU, oldestReservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		baseExpiration,
		2,
	)
	middleSKU, middleReservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		baseExpiration.Add(time.Hour),
		3,
	)
	newestSKU, newestReservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		baseExpiration.Add(2*time.Hour),
		4,
	)

	// Act
	result, err := service.ProcessExpiredReservations(ctx, 2)

	// Assert
	if err != nil {
		t.Fatalf("ProcessExpiredReservations() erro = %v; esperado nil", err)
	}
	want := services.ExpiredReservationsBatchResult{
		Selected:  2,
		Processed: 2,
	}
	if result != want {
		t.Errorf("ProcessExpiredReservations() resultado = %+v; esperado %+v", result, want)
	}
	assertReservationStatus(
		t,
		pool,
		ctx,
		oldestReservationID,
		database.InventoryReservationStatusReleased,
	)
	assertReservationStatus(
		t,
		pool,
		ctx,
		middleReservationID,
		database.InventoryReservationStatusReleased,
	)
	assertReservationStatus(
		t,
		pool,
		ctx,
		newestReservationID,
		database.InventoryReservationStatusActive,
	)
	assertInventoryState(t, pool, ctx, oldestSKU, 10, 0, 10)
	assertInventoryState(t, pool, ctx, middleSKU, 10, 0, 10)
	assertInventoryState(t, pool, ctx, newestSKU, 10, 4, 6)
}

func TestProcessExpiredReservationsIntegrationPartialFailure(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	baseExpiration := time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)

	validSKU, validReservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		baseExpiration.Add(time.Hour),
		4,
	)

	brokenSKU := newTestSKU(t, pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, 10)",
		brokenSKU,
	); err != nil {
		t.Fatalf("preparar estoque inconsistente: %v", err)
	}
	brokenReservationID := uuid.New()
	brokenExpiration := baseExpiration
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO inventory_reservations (
			reservation_id,
			order_id,
			sku_id,
			quantity,
			status,
			created_at,
			updated_at,
			expires_at
		) VALUES ($1, $2, $3, 4, 'active', $4, $4, $5)`,
		brokenReservationID,
		uuid.New(),
		brokenSKU,
		brokenExpiration.Add(-time.Hour),
		brokenExpiration,
	); err != nil {
		t.Fatalf("preparar reserva inconsistente: %v", err)
	}

	// Act
	result, err := service.ProcessExpiredReservations(ctx, 2)

	// Assert
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("ProcessExpiredReservations() erro = %v; esperado conter %v", err, pgx.ErrNoRows)
	}
	want := services.ExpiredReservationsBatchResult{
		Selected:  2,
		Processed: 1,
		Failed:    1,
	}
	if result != want {
		t.Errorf("ProcessExpiredReservations() resultado = %+v; esperado %+v", result, want)
	}
	assertReservationStatus(
		t,
		pool,
		ctx,
		validReservationID,
		database.InventoryReservationStatusReleased,
	)
	assertReservationStatus(
		t,
		pool,
		ctx,
		brokenReservationID,
		database.InventoryReservationStatusActive,
	)
	assertInventoryState(t, pool, ctx, validSKU, 10, 0, 10)
	assertInventoryState(t, pool, ctx, brokenSKU, 10, 0, 10)
}

func TestProcessExpiredReservationsIntegrationCanceledContext(t *testing.T) {
	// Arrange
	service, _, _ := setupIntegrationService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	result, err := service.ProcessExpiredReservations(ctx, 10)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ProcessExpiredReservations() erro = %v; esperado %v", err, context.Canceled)
	}
	if result != (services.ExpiredReservationsBatchResult{}) {
		t.Errorf(
			"ProcessExpiredReservations() resultado = %+v; esperado valor zero",
			result,
		)
	}
}

func TestProcessExpiredReservationsIntegrationConcurrent(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
		4,
	)

	type processResult struct {
		batch services.ExpiredReservationsBatchResult
		err   error
	}

	const workers = 2
	start := make(chan struct{})
	results := make(chan processResult, workers)
	for range workers {
		go func() {
			<-start
			batch, err := service.ProcessExpiredReservations(ctx, 1)
			results <- processResult{batch: batch, err: err}
		}()
	}

	// Act
	close(start)

	// Assert
	totalSelected := 0
	totalProcessed := 0
	totalFailed := 0
	for range workers {
		select {
		case result := <-results:
			if result.err != nil {
				t.Errorf(
					"ProcessExpiredReservations() concorrente erro = %v; esperado nil",
					result.err,
				)
			}
			totalSelected += result.batch.Selected
			totalProcessed += result.batch.Processed
			totalFailed += result.batch.Failed
		case <-ctx.Done():
			t.Fatalf("tempo esgotado aguardando processadores concorrentes: %v", ctx.Err())
		}
	}

	if totalSelected < 1 || totalSelected > workers {
		t.Errorf(
			"total selecionado pelos processadores = %d; esperado entre 1 e %d",
			totalSelected,
			workers,
		)
	}
	if totalProcessed != totalSelected {
		t.Errorf(
			"total processado = %d; esperado igual ao total selecionado %d",
			totalProcessed,
			totalSelected,
		)
	}
	if totalFailed != 0 {
		t.Errorf("total de falhas = %d; esperado 0", totalFailed)
	}
	assertReservationStatus(
		t,
		pool,
		ctx,
		reservationID,
		database.InventoryReservationStatusReleased,
	)
	assertInventoryState(t, pool, ctx, skuID, 10, 0, 10)
}
