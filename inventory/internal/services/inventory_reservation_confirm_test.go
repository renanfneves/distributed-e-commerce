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

func TestConfirmInventoryReservationIntegrationSuccess(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		time.Now().Add(time.Hour),
		4,
	)
	created, err := database.New(pool).GetInventoryReservationByID(ctx, reservationID)
	if err != nil {
		t.Fatalf("consultar reserva preparada: %v", err)
	}

	// Act
	confirmed, err := service.ConfirmInventoryReservation(ctx, reservationID)

	// Assert
	if err != nil {
		t.Fatalf("ConfirmInventoryReservation() erro = %v; esperado nil", err)
	}
	if confirmed.ReservationID != reservationID {
		t.Errorf("reservation_id = %s; esperado %s", confirmed.ReservationID, reservationID)
	}
	if confirmed.Status != database.InventoryReservationStatusConfirmed {
		t.Errorf(
			"status = %s; esperado %s",
			confirmed.Status,
			database.InventoryReservationStatusConfirmed,
		)
	}
	if !confirmed.UpdatedAt.Valid || !created.UpdatedAt.Valid {
		t.Fatal("updated_at inválido; esperado timestamps válidos")
	}
	if !confirmed.UpdatedAt.Time.After(created.UpdatedAt.Time) {
		t.Errorf(
			"updated_at confirmado = %v; esperado posterior a %v",
			confirmed.UpdatedAt.Time,
			created.UpdatedAt.Time,
		)
	}
	assertReservationStatus(
		t,
		pool,
		ctx,
		reservationID,
		database.InventoryReservationStatusConfirmed,
	)
	assertInventoryState(t, pool, ctx, skuID, 6, 0, 6)
}

func TestConfirmInventoryReservationIntegrationIdempotentRetry(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		time.Now().Add(time.Hour),
		4,
	)

	// Act
	first, firstErr := service.ConfirmInventoryReservation(ctx, reservationID)
	second, secondErr := service.ConfirmInventoryReservation(ctx, reservationID)

	// Assert
	if firstErr != nil {
		t.Fatalf("primeira confirmação erro = %v; esperado nil", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("retry da confirmação erro = %v; esperado nil", secondErr)
	}
	if first != second {
		t.Errorf("retry retornou reserva diferente: primeira=%+v retry=%+v", first, second)
	}
	assertInventoryState(t, pool, ctx, skuID, 6, 0, 6)
}

func TestConfirmInventoryReservationInvalidInput(t *testing.T) {
	// Arrange
	service := services.NewInventoryService(nil, testReservationTTL)

	// Act
	reservation, err := service.ConfirmInventoryReservation(context.Background(), uuid.Nil)

	// Assert
	if !errors.Is(err, services.ErrInvalidReservationInput) {
		t.Errorf("ConfirmInventoryReservation() erro = %v; esperado %v", err, services.ErrInvalidReservationInput)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf("reserva = %+v; esperado valor zero", reservation)
	}
}

func TestConfirmInventoryReservationIntegrationNotFound(t *testing.T) {
	// Arrange
	service, _, ctx := setupIntegrationService(t)

	// Act
	reservation, err := service.ConfirmInventoryReservation(ctx, uuid.New())

	// Assert
	if !errors.Is(err, services.ErrInventoryReservationNotFound) {
		t.Errorf("ConfirmInventoryReservation() erro = %v; esperado %v", err, services.ErrInventoryReservationNotFound)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf("reserva = %+v; esperado valor zero", reservation)
	}
}

func TestConfirmInventoryReservationIntegrationExpired(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		time.Now().Add(-time.Hour),
		4,
	)

	// Act
	reservation, err := service.ConfirmInventoryReservation(ctx, reservationID)

	// Assert
	if !errors.Is(err, services.ErrInventoryReservationExpired) {
		t.Errorf("ConfirmInventoryReservation() erro = %v; esperado %v", err, services.ErrInventoryReservationExpired)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf("reserva = %+v; esperado valor zero", reservation)
	}
	assertReservationStatus(t, pool, ctx, reservationID, database.InventoryReservationStatusActive)
	assertInventoryState(t, pool, ctx, skuID, 10, 4, 6)
}

func TestConfirmInventoryReservationIntegrationReleasedConflict(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		time.Now().Add(time.Hour),
		4,
	)
	if _, err := service.ReleaseInventoryReservation(ctx, reservationID); err != nil {
		t.Fatalf("preparar reserva liberada: %v", err)
	}

	// Act
	reservation, err := service.ConfirmInventoryReservation(ctx, reservationID)

	// Assert
	if !errors.Is(err, services.ErrInventoryReservationTransitionConflict) {
		t.Errorf(
			"ConfirmInventoryReservation() erro = %v; esperado %v",
			err,
			services.ErrInventoryReservationTransitionConflict,
		)
	}
	if reservation != (database.InventoryReservation{}) {
		t.Errorf("reserva = %+v; esperado valor zero", reservation)
	}
	assertReservationStatus(t, pool, ctx, reservationID, database.InventoryReservationStatusReleased)
	assertInventoryState(t, pool, ctx, skuID, 10, 0, 10)
}

func TestConfirmAndReleaseInventoryReservationIntegrationConcurrent(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID, reservationID := prepareReservationWithExpiration(
		t,
		service,
		pool,
		ctx,
		time.Now().Add(time.Hour),
		4,
	)

	type transitionResult struct {
		reservation database.InventoryReservation
		err         error
	}

	start := make(chan struct{})
	results := make(chan transitionResult, 2)
	go func() {
		<-start
		reservation, err := service.ConfirmInventoryReservation(ctx, reservationID)
		results <- transitionResult{reservation: reservation, err: err}
	}()
	go func() {
		<-start
		reservation, err := service.ReleaseInventoryReservation(ctx, reservationID)
		results <- transitionResult{reservation: reservation, err: err}
	}()
	// Act
	close(start)

	// Assert
	var successful database.InventoryReservation
	successes := 0
	conflicts := 0
	for range 2 {
		select {
		case result := <-results:
			switch {
			case result.err == nil:
				successes++
				successful = result.reservation
			case errors.Is(result.err, services.ErrInventoryReservationTransitionConflict):
				conflicts++
			default:
				t.Errorf("erro concorrente = %v; esperado nil ou conflito de transição", result.err)
			}
		case <-ctx.Done():
			t.Fatalf("tempo esgotado aguardando transições concorrentes: %v", ctx.Err())
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf(
			"resultado concorrente = (sucessos=%d, conflitos=%d); esperado (1, 1)",
			successes,
			conflicts,
		)
	}

	switch successful.Status {
	case database.InventoryReservationStatusConfirmed:
		assertInventoryState(t, pool, ctx, skuID, 6, 0, 6)
	case database.InventoryReservationStatusReleased:
		assertInventoryState(t, pool, ctx, skuID, 10, 0, 10)
	default:
		t.Fatalf("status terminal = %s; esperado confirmed ou released", successful.Status)
	}
	assertReservationStatus(t, pool, ctx, reservationID, successful.Status)
}
