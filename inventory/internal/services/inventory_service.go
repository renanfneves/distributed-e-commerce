package services

import (
	"context"
	"e-commerce/inventory/internal/database"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type InventoryService struct {
	queries *database.Queries
}

var ErrInventoryItemNotFound = errors.New("inventory item not found")
var ErrInvalidQuantity = errors.New("invalid quantity")
var ErrInventoryItemAlreadyExists = errors.New("inventory item already exists")

func NewInventoryService(queries *database.Queries) InventoryService {
	return InventoryService{
		queries: queries,
	}
}

type InventoryItemInput struct {
	SkuID          uuid.UUID `json:"sku_id"`
	QuantityOnHand int64     `json:"quantity_on_hand"`
}

func (s InventoryService) CreateInventoryItem(ctx context.Context, i InventoryItemInput) (uuid.UUID, error) {
	if i.QuantityOnHand < 0 {
		return uuid.Nil, ErrInvalidQuantity
	}
	_, err := s.queries.CreateInventoryItem(ctx, database.CreateInventoryItemParams{
		SkuID:          i.SkuID,
		QuantityOnHand: i.QuantityOnHand,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "inventory_items_pkey" {
			return uuid.Nil, ErrInventoryItemAlreadyExists
		}
		return uuid.Nil, err
	}
	return i.SkuID, nil
}

func (s InventoryService) GetInventoryItemBySKU(ctx context.Context, id uuid.UUID) (database.GetInventoryItemBySKURow, error) {
	item, err := s.queries.GetInventoryItemBySKU(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.GetInventoryItemBySKURow{}, ErrInventoryItemNotFound
		}
		return database.GetInventoryItemBySKURow{}, err
	}
	return item, nil
}
