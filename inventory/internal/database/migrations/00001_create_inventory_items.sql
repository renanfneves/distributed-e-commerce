-- +goose Up
CREATE TABLE inventory_items (
    sku_id UUID PRIMARY KEY,
    quantity_on_hand BIGINT NOT NULL,
    quantity_reserved BIGINT NOT NULL DEFAULT 0,

    CONSTRAINT quantity_on_hand_nonnegative
        CHECK (quantity_on_hand >= 0),

    CONSTRAINT quantity_reserved_nonnegative
        CHECK (quantity_reserved >= 0),

    CONSTRAINT quantity_on_hand_higher_than_quantity_reserved
        CHECK (quantity_reserved <= quantity_on_hand)
);

-- +goose Down
DROP TABLE inventory_items;
