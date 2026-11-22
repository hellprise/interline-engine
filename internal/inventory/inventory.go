// Package inventory defines the interface for working with flight inventory (seats).
package inventory

import (
	"context"
	"errors"
)

// CabinSeats represents seat capacity for a single cabin on a flight.
type CabinSeats struct {
	Cabin string
	Total int
	Held  int
}

// Available returns the number of seats available to hold.
func (c CabinSeats) Available() int {
	return c.Total - c.Held
}

var (
	// ErrFlightNotFound is returned when the flight or cabin does not exist.
	ErrFlightNotFound = errors.New("flight or cabin not found")
	// ErrNotEnoughSeats is returned when fewer seats remain than requested.
	ErrNotEnoughSeats = errors.New("not enough seats")
	// ErrInvalidSeatCount is returned when the requested seat count is not positive.
	ErrInvalidSeatCount = errors.New("seat count must be a positive value")
)

// Repository defines storage operations for flight seat inventory.
type Repository interface {
	// FlightSeats returns cabins for a flight in alphabetical order or ErrFlightNotFound if none exist.
	FlightSeats(ctx context.Context, flightKey string) ([]CabinSeats, error)
	// Hold reserves n seats in a cabin, returning ErrNotEnoughSeats, ErrFlightNotFound or ErrInvalidSeatCount.
	Hold(ctx context.Context, flightKey, cabin string, n int) error
}
