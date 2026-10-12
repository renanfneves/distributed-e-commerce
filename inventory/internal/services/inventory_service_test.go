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
		if _, err := pool.Exec(ctx, "DELETE FROM inventory_items WHERE sku_id = $1", id); err != nil {
			t.Errorf("limpar item de teste: %v", err)
		}
	})
}

func TestCreateInventoryItemIntegrationSuccess(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewInventoryService(database.New(pool))
	for _, quantity := range []int64{0, 10, 1 << 33} {
		t.Run(fmt.Sprintf("quantity_%d", quantity), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			skuID := uuid.New()
			cleanupInventory(t, pool, skuID)
			id, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: quantity})
			if err != nil || id != skuID {
				t.Fatalf("criar item: id=%s err=%v", id, err)
			}
			var onHand, reserved int64
			if err := pool.QueryRow(ctx, "SELECT quantity_on_hand, quantity_reserved FROM inventory_items WHERE sku_id = $1", skuID).Scan(&onHand, &reserved); err != nil {
				t.Fatalf("consultar persistência: %v", err)
			}
			if onHand != quantity || reserved != 0 {
				t.Errorf("quantidades persistidas incorretas: on_hand=%d reserved=%d", onHand, reserved)
			}
			item, err := service.GetInventoryItemBySKU(ctx, skuID)
			if err != nil {
				t.Fatalf("consultar pelo serviço: %v", err)
			}
			if item.SkuID != skuID || item.QuantityOnHand != quantity || item.QuantityReserved != 0 || item.QuantityAvailable != quantity {
				t.Errorf("item retornado incorreto: %+v", item)
			}
		})
	}
}

func TestCreateInventoryItemIntegrationDuplicate(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewInventoryService(database.New(pool))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	skuID := uuid.New()
	cleanupInventory(t, pool, skuID)
	if _, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: 10}); err != nil {
		t.Fatalf("preparar item original: %v", err)
	}
	id, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: 99})
	if !errors.Is(err, services.ErrInventoryItemAlreadyExists) || id != uuid.Nil {
		t.Fatalf("esperado erro de duplicidade e ID vazio: id=%s err=%v", id, err)
	}
	var quantity int64
	if err := pool.QueryRow(ctx, "SELECT quantity_on_hand FROM inventory_items WHERE sku_id = $1", skuID).Scan(&quantity); err != nil {
		t.Fatalf("verificar original: %v", err)
	}
	if quantity != 10 {
		t.Errorf("original alterado: quantity=%d", quantity)
	}
}

func TestCreateInventoryItemInvalidQuantity(t *testing.T) {
	// Sem queries: a validação deve rejeitar a entrada antes de acessar o banco.
	service := services.NewInventoryService(nil)
	id, err := service.CreateInventoryItem(context.Background(), services.InventoryItemInput{SkuID: uuid.New(), QuantityOnHand: -1})
	if !errors.Is(err, services.ErrInvalidQuantity) || id != uuid.Nil {
		t.Fatalf("esperado erro de quantidade e ID vazio: id=%s err=%v", id, err)
	}
}

func TestGetInventoryItemIntegrationNotFound(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewInventoryService(database.New(pool))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	item, err := service.GetInventoryItemBySKU(ctx, uuid.New())
	if !errors.Is(err, services.ErrInventoryItemNotFound) {
		t.Fatalf("esperado erro de ausência, recebido: %v", err)
	}
	if item != (database.GetInventoryItemBySKURow{}) {
		t.Errorf("esperado retorno vazio: %+v", item)
	}
}

func TestGetInventoryItemIntegrationAvailability(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewInventoryService(database.New(pool))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	skuID := uuid.New()
	cleanupInventory(t, pool, skuID)
	if _, err := pool.Exec(ctx, "INSERT INTO inventory_items (sku_id, quantity_on_hand, quantity_reserved) VALUES ($1, 10, 4)", skuID); err != nil {
		t.Fatalf("preparar estoque reservado: %v", err)
	}
	item, err := service.GetInventoryItemBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("consultar item: %v", err)
	}
	if item.SkuID != skuID || item.QuantityOnHand != 10 || item.QuantityReserved != 4 || item.QuantityAvailable != 6 {
		t.Errorf("disponibilidade incorreta: %+v", item)
	}
}
