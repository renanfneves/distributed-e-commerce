-- +goose Up
ALTER TABLE inventory_reservations
    ADD COLUMN order_id UUID NOT NULL;

CREATE INDEX inventory_reservations_order_id_idx
    ON inventory_reservations (order_id);


-- +goose Down
DROP INDEX inventory_reservations_order_id_idx;

ALTER TABLE inventory_reservations
DROP COLUMN order_id;
