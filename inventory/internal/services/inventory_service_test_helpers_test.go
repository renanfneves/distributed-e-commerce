package services_test

import (
	"context"
	"os"
	"testing"
	"time"

	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/services"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testReservationTTL = 15 * time.Minute

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
	service := services.NewInventoryService(pool, testReservationTTL)
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

func prepareReservationWithExpiration(t *testing.T, service services.InventoryService, pool *pgxpool.Pool, ctx context.Context, expiresAt time.Time, quantity int64) (uuid.UUID, uuid.UUID) {
	t.Helper()

	skuID := newTestSKU(t, pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO inventory_items (sku_id, quantity_on_hand) VALUES ($1, 10)",
		skuID,
	); err != nil {
		t.Fatalf("preparar estoque: %v", err)
	}

	reservationID := uuid.New()
	if _, err := service.CreateInventoryReservation(ctx, services.InventoryReservationInput{
		ReservationID: reservationID,
		OrderID:       uuid.New(),
		SkuID:         skuID,
		Quantity:      quantity,
	}); err != nil {
		t.Fatalf("preparar reserva: %v", err)
	}

	createdAt := expiresAt.Add(-time.Hour)
	if _, err := pool.Exec(
		ctx,
		`UPDATE inventory_reservations
		SET created_at = $2, updated_at = $2, expires_at = $3
		WHERE reservation_id = $1`,
		reservationID,
		createdAt,
		expiresAt,
	); err != nil {
		t.Fatalf("configurar prazo da reserva: %v", err)
	}

	return skuID, reservationID
}

func assertReservationStatus(t *testing.T, pool *pgxpool.Pool, ctx context.Context, reservationID uuid.UUID, want database.InventoryReservationStatus) {
	t.Helper()

	var got database.InventoryReservationStatus
	if err := pool.QueryRow(
		ctx,
		"SELECT status FROM inventory_reservations WHERE reservation_id = $1",
		reservationID,
	).Scan(&got); err != nil {
		t.Fatalf("consultar status da reserva %s: %v", reservationID, err)
	}

	if got != want {
		t.Errorf("status da reserva %s = %s; esperado %s", reservationID, got, want)
	}
}
