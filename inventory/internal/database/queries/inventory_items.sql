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