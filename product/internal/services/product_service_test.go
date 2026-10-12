package services_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"e-commerce/product/internal/database"
	"e-commerce/product/internal/services"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	uri := os.Getenv("TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("defina TEST_DATABASE_URL para executar o teste de integração")
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

func cleanupProducts(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM product_skus WHERE product_id IN
			(SELECT id FROM products WHERE name = ANY($1::text[]))`, names); err != nil {
			t.Errorf("limpar SKUs: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM products WHERE name = ANY($1::text[])`, names); err != nil {
			t.Errorf("limpar produtos: %v", err)
		}
	})
}

func TestCreateProductIntegrationSuccess(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewProductService(pool, database.New(pool))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	suffix := uuid.NewString()
	input := services.ProductInput{
		Name: "teste-produto-" + suffix, Description: "Produto de integração",
		Code: "teste-sku-" + suffix, PriceInCents: 5990, Currency: "USD",
	}
	cleanupProducts(t, pool, input.Name)

	id, err := service.CreateProduct(ctx, input)
	if err != nil {
		t.Fatalf("criar produto: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("esperado ID não vazio")
	}

	var name, description string
	if err := pool.QueryRow(ctx, `SELECT name, description FROM products WHERE id = $1`, id).
		Scan(&name, &description); err != nil {
		t.Fatalf("consultar produto persistido: %v", err)
	}
	if name != input.Name || description != input.Description {
		t.Errorf("produto incorreto: name=%q description=%q", name, description)
	}
	var code, currency string
	var price int64
	if err := pool.QueryRow(ctx, `SELECT code, price_cents, currency FROM product_skus WHERE product_id = $1`, id).
		Scan(&code, &price, &currency); err != nil {
		t.Fatalf("consultar SKU associado: %v", err)
	}
	if code != input.Code || price != input.PriceInCents || currency != input.Currency {
		t.Errorf("SKU incorreto: code=%q price=%d currency=%q", code, price, currency)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM product_skus WHERE product_id = $1`, id).Scan(&count); err != nil {
		t.Fatalf("contar SKUs: %v", err)
	}
	if count != 1 {
		t.Errorf("esperado 1 SKU, recebido %d", count)
	}
}

func TestCreateProductIntegrationRollback(t *testing.T) {
	pool := integrationPool(t)
	service := services.NewProductService(pool, database.New(pool))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	suffix := uuid.NewString()
	first := services.ProductInput{
		Name: "teste-original-" + suffix, Description: "Original",
		Code: "teste-duplicado-" + suffix, PriceInCents: 1000, Currency: "BRL",
	}
	second := first
	second.Name = "teste-rollback-" + suffix
	cleanupProducts(t, pool, first.Name, second.Name)

	firstID, err := service.CreateProduct(ctx, first)
	if err != nil {
		t.Fatalf("preparar produto original: %v", err)
	}
	id, err := service.CreateProduct(ctx, second)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "product_skus_code_unique" {
		t.Fatalf("esperada violação de unicidade do SKU, recebido: %v", err)
	}
	if id != uuid.Nil {
		t.Errorf("esperado ID vazio na falha, recebido %s", id)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products WHERE name = $1`, second.Name).Scan(&count); err != nil {
		t.Fatalf("verificar rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("rollback falhou: %d produto(s) da operação rejeitada persistiram", count)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM product_skus WHERE code = $1`, first.Code).Scan(&count); err != nil {
		t.Fatalf("verificar SKUs: %v", err)
	}
	if count != 1 {
		t.Errorf("esperado apenas o SKU original, recebido %d", count)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products p JOIN product_skus s ON s.product_id = p.id
		WHERE p.id = $1 AND p.name = $2 AND s.code = $3`, firstID, first.Name, first.Code).Scan(&count); err != nil {
		t.Fatalf("verificar preservação do original: %v", err)
	}
	if count != 1 {
		t.Error("produto e SKU originais não foram preservados")
	}
}
