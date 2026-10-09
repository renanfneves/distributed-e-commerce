-- name: CreateProduct :one
INSERT INTO products
    (name, description)
VALUES ($1, $2)
    RETURNING id;

-- name: CreateProductSKU :one
INSERT INTO product_skus
    (product_id, code, price_cents, currency)
VALUES
    ($1, $2, $3, $4)
    RETURNING id;

-- name: GetProductByID :one
SELECT
    p.id as product_id,
    ps.id as product_sku_id,
    p.name,
    p.description,
    ps.code,
    ps.price_cents,
    ps.currency
FROM
    products p
INNER JOIN
    product_skus ps
ON
    p.id = ps.product_id
WHERE
    p.id = $1;

-- name: ListProducts :many
SELECT
    p.id as product_id,
    ps.id as product_sku_id,
    p.name,
    p.description,
    ps.code,
    ps.price_cents,
    ps.currency
FROM
    products p
INNER JOIN
    product_skus ps
ON
    p.id = ps.product_id
ORDER BY
    p.created_at DESC, p.id DESC
LIMIT $1
OFFSET $2;


