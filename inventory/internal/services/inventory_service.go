package services

import (
	"context"
	"e-commerce/inventory/internal/database"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InventoryService struct {
	pool    *pgxpool.Pool
	queries *database.Queries
}

var ErrInventoryItemNotFound = errors.New("inventory item not found")
var ErrInvalidQuantity = errors.New("invalid quantity")
var ErrInventoryItemAlreadyExists = errors.New("inventory item already exists")
var ErrInventoryReservationRejected = errors.New("inventory reservation rejected")
var ErrInvalidReservationInput = errors.New("invalid reservation input")
var ErrReservationIdempotencyConflict = errors.New("reservation idempotency conflict")

func NewInventoryService(pool *pgxpool.Pool) InventoryService {
	return InventoryService{
		pool:    pool,
		queries: database.New(pool),
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

func (s InventoryService) ReserveInventory(ctx context.Context, skuID uuid.UUID, quantity int64) (database.ReserveInventoryRow, error) {
	if quantity <= 0 {
		return database.ReserveInventoryRow{}, ErrInvalidQuantity
	}
	rows, err := s.queries.ReserveInventory(ctx, database.ReserveInventoryParams{
		SkuID:            skuID,
		QuantityReserved: quantity,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ReserveInventoryRow{}, ErrInventoryReservationRejected
		}
		return database.ReserveInventoryRow{}, err
	}
	return rows, nil
}

type InventoryReservationInput struct {
	ReservationID uuid.UUID
	OrderID       uuid.UUID
	SkuID         uuid.UUID
	Quantity      int64
}

func (s InventoryService) CreateInventoryReservation(ctx context.Context, input InventoryReservationInput) (database.InventoryReservation, error) {
	if input.ReservationID == uuid.Nil ||
		input.OrderID == uuid.Nil ||
		input.SkuID == uuid.Nil ||
		input.Quantity <= 0 {
		return database.InventoryReservation{}, ErrInvalidReservationInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return database.InventoryReservation{}, err
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	reservation, err := txQueries.CreateInventoryReservation(
		ctx,
		database.CreateInventoryReservationParams{
			ReservationID: input.ReservationID,
			OrderID:       input.OrderID,
			SkuID:         input.SkuID,
			Quantity:      input.Quantity,
		},
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr :=
				txQueries.GetInventoryReservationByID(
					ctx,
					input.ReservationID,
				)
			if getErr != nil {
				return database.InventoryReservation{}, getErr
			}
			isSameRequest :=
				existing.OrderID == input.OrderID &&
					existing.SkuID == input.SkuID &&
					existing.Quantity == input.Quantity

			if !isSameRequest {
				return database.InventoryReservation{},
					ErrReservationIdempotencyConflict
			}

			if err := tx.Commit(ctx); err != nil {
				return database.InventoryReservation{}, err
			}

			return existing, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23503" &&
			pgErr.ConstraintName ==
				"inventory_reservations_sku_id_fkey" {
			return database.InventoryReservation{},
				ErrInventoryReservationRejected
		}

		return database.InventoryReservation{}, err
	}
	_, err = txQueries.ReserveInventory(ctx, database.ReserveInventoryParams{
		SkuID:            input.SkuID,
		QuantityReserved: input.Quantity,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.InventoryReservation{},
				ErrInventoryReservationRejected
		}

		return database.InventoryReservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return database.InventoryReservation{}, err
	}
	return reservation, nil
}
