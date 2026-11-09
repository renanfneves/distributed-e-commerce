-- +goose Up
ALTER TABLE inventory_reservations
    ADD COLUMN expires_at TIMESTAMPTZ;

UPDATE inventory_reservations
    SET expires_at = created_at + INTERVAL '15 minutes';

ALTER TABLE inventory_reservations
    ALTER COLUMN expires_at SET NOT NULL;

ALTER TABLE inventory_reservations
    ADD CONSTRAINT inventory_reservation_expiration_after_creation
        CHECK (expires_at > created_at);

CREATE INDEX inventory_reservations_active_expires_at_idx
    ON inventory_reservations (expires_at)
        WHERE status = 'active';

-- +goose Down
DROP INDEX inventory_reservations_active_expires_at_idx;

ALTER TABLE inventory_reservations
    DROP COLUMN expires_at;
