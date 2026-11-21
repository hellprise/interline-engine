-- +goose Up
CREATE TABLE seat_inventory (
  flight_key TEXT NOT NULL,
  cabin TEXT NOT NULL,
  seats_held INT NOT NULL DEFAULT 0,
  seats_total INT NOT NULL,
  PRIMARY KEY (flight_key, cabin),
  CONSTRAINT seat_inventory_held_range CHECK (
    seats_held >= 0
    AND seats_held <= seats_total
  )
);
-- +goose Down
DROP TABLE seat_inventory;