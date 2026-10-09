package services

import (
	"context"
	"e-commerce/product/internal/database"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductService struct {
	pool    *pgxpool.Pool
	queries *database.Queries
}

var ErrProductNotFound = errors.New("product not found")

func NewProductService(pool *pgxpool.Pool, queries *database.Queries) ProductService {
	return ProductService{
		pool:    pool,
		queries: queries,
	}
}

type ProductInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Code         string `json:"code"`
	PriceInCents int64  `json:"price_in_cents"`
	Currency     string `json:"currency"`
}

func (s ProductService) CreateProduct(ctx context.Context, p ProductInput) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	tQuery := s.queries.WithTx(tx)
	productId, prodErr := tQuery.CreateProduct(ctx, database.CreateProductParams{
		Name:        p.Name,
		Description: p.Description,
	})
	if prodErr != nil {
		return uuid.Nil, prodErr
	}
	_, skuErr := tQuery.CreateProductSKU(ctx, database.CreateProductSKUParams{
		ProductID:  productId,
		Code:       p.Code,
		PriceCents: p.PriceInCents,
		Currency:   p.Currency,
	})
	if skuErr != nil {
		return uuid.Nil, skuErr
	}
	commitErr := tx.Commit(ctx)
	if commitErr != nil {
		return uuid.Nil, commitErr
	}

	return productId, nil
}

func (s ProductService) GetProductById(ctx context.Context, id uuid.UUID) (database.GetProductByIDRow, error) {
	product, err := s.queries.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.GetProductByIDRow{}, ErrProductNotFound
		}
		return database.GetProductByIDRow{}, err
	}

	return product, nil

}

func (s ProductService) ListProducts(ctx context.Context, offset int32, limit int32) ([]database.ListProductsRow, error) {
	products, err := s.queries.ListProducts(ctx, database.ListProductsParams{
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return []database.ListProductsRow{}, err
	}
	return products, nil
}
