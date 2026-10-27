-- name: CreateInventoryItem :one
INSERT INTO inventory_items
    (sku_id, quantity_on_hand)
VALUES
    ($1, $2)
RETURNING
    sku_id,
    quantity_on_hand,
    quantity_reserved,
    (quantity_on_hand - quantity_reserved)::bigint AS quantity_available;

-- name: GetInventoryItemBySKU :one
SELECT
    sku_id,
    quantity_on_hand,
    quantity_reserved,
    (quantity_on_hand - quantity_reserved)::bigint AS quantity_available
FROM
    inventory_items
WHERE
    sku_id = $1;

-- name: ReserveInventory :one
UPDATE
    inventory_items
SET
    quantity_reserved = quantity_reserved + $1
WHERE
    sku_id = $2
    AND $1::bigint > 0
    AND quantity_on_hand - quantity_reserved >= $1
RETURNING
    sku_id,
    quantity_on_hand,
    quantity_reserved,
    (quantity_on_hand - quantity_reserved)::bigint AS quantity_available;
