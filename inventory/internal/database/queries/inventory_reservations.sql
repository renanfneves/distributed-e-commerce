-- name: CreateInventoryReservation :one
INSERT INTO inventory_reservations (
    reservation_id,
    order_id,
    sku_id,
    quantity
)
VALUES (
    $1,
    $2,
    $3,
    $4
)
ON CONFLICT (reservation_id) DO NOTHING
RETURNING
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id;

-- name: GetInventoryReservationByID :one
SELECT
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id
FROM
    inventory_reservations
WHERE
    reservation_id = $1;

-- name: GetInventoryReservationByIDForUpdate :one
SELECT
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id
FROM
    inventory_reservations
WHERE
    reservation_id = $1
FOR UPDATE;

-- name: MarkInventoryReservationReleased :one
UPDATE
    inventory_reservations
SET
    status = 'released',
    updated_at = NOW()
WHERE
    reservation_id = $1
  AND status = 'active'
RETURNING
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id;