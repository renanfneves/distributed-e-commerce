-- +goose Up
CREATE TYPE inventory_reservation_status AS ENUM (
    'active',
    'confirmed',
    'released'
);

CREATE TABLE inventory_reservations (
    reservation_id UUID PRIMARY KEY,
    sku_id UUID NOT NULL REFERENCES inventory_items(sku_id),
    quantity BIGINT NOT NULL,
    status inventory_reservation_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT reservation_quantity_positive
        CHECK (quantity > 0)
);

CREATE INDEX inventory_reservations_sku_id_idx
    ON inventory_reservations (sku_id);

-- +goose Down
DROP TABLE inventory_reservations;
DROP TYPE inventory_reservation_status;
