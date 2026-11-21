// Package inventory defines the interface for working with flight inventory (seats)
package inventory

import (
	"context"
	"errors"
)

type CabinSeats struct {
	Cabin string
	Total int
	Held  int
}

func (c CabinSeats) Available() int {
	return c.Total - c.Held
}

var (
	ErrFlightNotFound   = errors.New("flight or cabin not found")
	ErrNotEnoughSeats   = errors.New("not enough seats")
	ErrInvalidSeatCount = errors.New("seats count must be a positive value")
)

type Repository interface {
	// FlightSeats - classes of flights by alphabet. no rows - ErrFlightNotFound
	FlightSeats(ctx context.Context, flightKey string) ([]CabinSeats, error)
	// Hold reserves n seats in a class. Not enough seats - ErrNotEnoughSeats, n <= 0 - ErrInvalidSeatCount
	Hold(ctx context.Context, flightKey, cabin string, n int) error
}
