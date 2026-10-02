package store

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("resource not found")
	ErrInsufficientShips = errors.New("insufficient ships")
	ErrInsufficientFuel  = errors.New("insufficient deuterium")
	ErrCannotRecall      = errors.New("fleet cannot be recalled")
)

type InsufficientShipsError struct {
	ShipCode  string
	Available int64
	Requested int64
}

func (e *InsufficientShipsError) Error() string {
	return fmt.Sprintf("Insufficient ships for code %s (available: %d, requested: %d)", e.ShipCode, e.Available, e.Requested)
}

func (e *InsufficientShipsError) Is(target error) bool {
	return target == ErrInsufficientShips
}

type InsufficientFuelError struct {
	Needed int64
}

func (e *InsufficientFuelError) Error() string {
	return fmt.Sprintf("Insufficient deuterium for flight fuel (needed: %d)", e.Needed)
}

func (e *InsufficientFuelError) Is(target error) bool {
	return target == ErrInsufficientFuel
}
