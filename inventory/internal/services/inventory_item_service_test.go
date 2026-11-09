package services_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"e-commerce/inventory/internal/database"
	"e-commerce/inventory/internal/services"

	"github.com/google/uuid"
)

func TestCreateInventoryItemIntegrationSuccess(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewInventoryService(pool, testReservationTTL)
	for _, quantity := range []int64{0, 10, 1 << 33} {
		t.Run(fmt.Sprintf("quantity_%d", quantity), func(t *testing.T) {
			// Arrange
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			skuID := uuid.New()
			cleanupInventory(t, pool, skuID)

			// Act
			id, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: quantity})

			// Assert
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
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)

	if _, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: 10}); err != nil {
		t.Fatalf("preparar item original: %v", err)
	}
	// Act
	id, err := service.CreateInventoryItem(ctx, services.InventoryItemInput{SkuID: skuID, QuantityOnHand: 99})

	// Assert
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
	// Arrange
	service := services.NewInventoryService(nil, testReservationTTL)

	// Act
	id, err := service.CreateInventoryItem(context.Background(), services.InventoryItemInput{SkuID: uuid.New(), QuantityOnHand: -1})

	// Assert
	if !errors.Is(err, services.ErrInvalidQuantity) {
		t.Errorf("CreateInventoryItem() erro = %v; esperado %v", err, services.ErrInvalidQuantity)
	}
	if id != uuid.Nil {
		t.Errorf("CreateInventoryItem() id = %s; esperado UUID vazio", id)
	}
}

func TestGetInventoryItemIntegrationNotFound(t *testing.T) {
	// Arrange
	service, _, ctx := setupIntegrationService(t)

	// Act
	item, err := service.GetInventoryItemBySKU(ctx, uuid.New())

	// Assert
	if !errors.Is(err, services.ErrInventoryItemNotFound) {
		t.Errorf("GetInventoryItemBySKU() erro = %v; esperado %v", err, services.ErrInventoryItemNotFound)
	}
	if item != (database.GetInventoryItemBySKURow{}) {
		t.Errorf("GetInventoryItemBySKU() item = %+v; esperado valor zero", item)
	}
}

func TestGetInventoryItemIntegrationAvailability(t *testing.T) {
	// Arrange
	service, pool, ctx := setupIntegrationService(t)
	skuID := newTestSKU(t, pool)

	if _, err := pool.Exec(ctx, "INSERT INTO inventory_items (sku_id, quantity_on_hand, quantity_reserved) VALUES ($1, 10, 4)", skuID); err != nil {
		t.Fatalf("preparar estoque reservado: %v", err)
	}
	// Act
	item, err := service.GetInventoryItemBySKU(ctx, skuID)

	// Assert
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
	// Arrange
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

	// Act
	close(start)

	// Assert
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
