-- name: CreateInventoryReservation :one
INSERT INTO inventory_reservations (
    reservation_id,
    order_id,
    sku_id,
    quantity,
    expires_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    NOW() + sqlc.arg(ttl_seconds)::bigint * INTERVAL '1 second'
)
ON CONFLICT (reservation_id) DO NOTHING
RETURNING
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id,
    expires_at;

-- name: GetInventoryReservationByID :one
SELECT
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id,
    expires_at
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
    order_id,
    expires_at
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
    order_id,
    expires_at;

-- name: MarkInventoryReservationConfirmed :one
UPDATE
    inventory_reservations
SET
    status = 'confirmed',
    updated_at = NOW()
WHERE
    reservation_id = $1
    AND status = 'active'
    AND expires_at > NOW()
RETURNING
    reservation_id,
    sku_id,
    quantity,
    status,
    created_at,
    updated_at,
    order_id,
    expires_at;

-- name: ListExpiredActiveReservationIDs :many
SELECT
    reservation_id
FROM
    inventory_reservations
WHERE
    status = 'active'
    AND expires_at <= NOW()
ORDER BY
    expires_at ASC,
    reservation_id ASC
LIMIT sqlc.arg(batch_size)::int;
