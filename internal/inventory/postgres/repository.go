// Package postgres implements inventory.Repository backed by PostgreSQL.
package postgres

import (
	"context"
	"fmt"

	"github.com/hellprise/interline-engine/internal/inventory"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository stores and updates seat inventory in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository using the provided pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Check during compilation, if *Repository implements inventory.Repository.
var _ inventory.Repository = (*Repository)(nil)

const flightSeatsSQL = `
SELECT cabin, seats_total, seats_held
FROM seat_inventory
WHERE flight_key = $1
ORDER BY cabin;
`

// FlightSeats returns all cabins for flightKey in alphabetical order or inventory.ErrFlightNotFound.
func (r *Repository) FlightSeats(ctx context.Context, flightKey string) ([]inventory.CabinSeats, error) {
	rows, err := r.pool.Query(ctx, flightSeatsSQL, flightKey)
	if err != nil {
		return nil, fmt.Errorf("flight seats %s: %w", flightKey, err)
	}

	seats, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (inventory.CabinSeats, error) {
		var s inventory.CabinSeats
		err := row.Scan(&s.Cabin, &s.Total, &s.Held) // important: order as in SELECT in flightSeatsSQL query.
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("flight seats %s: %w", flightKey, err)
	}
	if len(seats) == 0 {
		return nil, inventory.ErrFlightNotFound
	}

	return seats, nil
}

// holdSQL checks free seats and updates seats_held in one atomic UPDATE so concurrent holds cannot race.
const holdSQL = `
UPDATE seat_inventory
SET seats_held = seats_held + $3
WHERE flight_key = $1 AND cabin = $2 AND $3 <= seats_total - seats_held
`

const cabinExistsSQL = `
SELECT EXISTS (SELECT 1 FROM seat_inventory WHERE flight_key = $1 AND cabin = $2)
`

// Hold reserves n seats in the given flight cabin using an atomic conditional UPDATE.
func (r *Repository) Hold(ctx context.Context, flightKey string, cabin string, n int) error {
	if n <= 0 {
		return inventory.ErrInvalidSeatCount
	}

	tag, err := r.pool.Exec(ctx, holdSQL, flightKey, cabin, n)
	if err != nil {
		return fmt.Errorf("hold %s/%s: %w", flightKey, cabin, err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	var exists bool
	if err := r.pool.QueryRow(ctx, cabinExistsSQL, flightKey, cabin).Scan(&exists); err != nil {
		return fmt.Errorf("hold %s/%s: %w", flightKey, cabin, err)
	}
	if !exists {
		return inventory.ErrFlightNotFound
	}

	return inventory.ErrNotEnoughSeats
}
