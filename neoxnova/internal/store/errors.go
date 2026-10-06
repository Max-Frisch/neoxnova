package store

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound               = errors.New("resource not found")
	ErrInsufficientShips      = errors.New("insufficient ships")
	ErrInsufficientFuel       = errors.New("insufficient deuterium")
	ErrCannotRecall           = errors.New("fleet cannot be recalled")
	ErrInsufficientResources  = errors.New("insufficient resources")
	ErrQueueBusy              = errors.New("build queue is busy")
	ErrPrerequisites          = errors.New("prerequisites not met")
	ErrNoFields               = errors.New("not enough fields")
	ErrUnknownCode            = errors.New("unknown build code")
	ErrInvalidQuantity        = errors.New("invalid quantity")
	ErrNoobProtection         = errors.New("target is protected by the noob-protection points ratio")
	ErrNoTarget               = errors.New("mission requires an existing target celestial")
	ErrUserExists             = errors.New("username or email already registered")
	ErrInvalidCredentials     = errors.New("invalid username or password")
	ErrCannotAbandonHome      = errors.New("the homeworld cannot be abandoned")
	ErrLastPlanet             = errors.New("cannot abandon your last planet")
	ErrFleetInbound           = errors.New("cannot abandon a planet with active fleets")
	ErrInvalidCoordinates     = errors.New("target coordinates are out of range")
	ErrSameCoordinates        = errors.New("planet is already at those coordinates")
	ErrTargetOccupied         = errors.New("target coordinates are occupied or reserved")
	ErrInsufficientDarkMatter = errors.New("insufficient dark matter")
	ErrRelocationCooldown     = errors.New("planet was relocated too recently")
	ErrAttackLocked           = errors.New("planet cannot attack yet after relocation")
)

// NoobProtectionRatio is the maximum allowed points ratio (in either direction)
// between attacker and defender, learned from the reference server (~4:1).
const NoobProtectionRatio = 4

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
