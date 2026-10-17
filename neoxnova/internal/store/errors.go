package store

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound                  = errors.New("resource not found")
	ErrInsufficientShips         = errors.New("insufficient ships")
	ErrStationaryShip            = errors.New("ships cannot be dispatched")
	ErrInsufficientFuel          = errors.New("insufficient deuterium")
	ErrCannotRecall              = errors.New("fleet cannot be recalled")
	ErrInsufficientResources     = errors.New("insufficient resources")
	ErrQueueBusy                 = errors.New("build queue is busy")
	ErrQueueFull                 = errors.New("build queue is full")
	ErrPrerequisites             = errors.New("prerequisites not met")
	ErrEnergyRequirement         = errors.New("not enough energy production for this research")
	ErrNoFields                  = errors.New("not enough fields")
	ErrUnknownCode               = errors.New("unknown build code")
	ErrInvalidQuantity           = errors.New("invalid quantity")
	ErrNoobProtection            = errors.New("target is protected by the noob-protection points ratio")
	ErrNoTarget                  = errors.New("mission requires an existing target celestial")
	ErrExpeditionHoldRequired    = errors.New("expedition requires a holding time (1-24 hours)")
	ErrExpeditionSlotsFull       = errors.New("all expedition slots are in use")
	ErrInvalidExpeditionTarget   = errors.New("expeditions must target the deep-space slot")
	ErrUserExists                = errors.New("username or email already registered")
	ErrInvalidCredentials        = errors.New("invalid username or password")
	ErrCannotAbandonHome         = errors.New("the homeworld cannot be abandoned")
	ErrLastPlanet                = errors.New("cannot abandon your last planet")
	ErrFleetInbound              = errors.New("cannot abandon a planet with active fleets")
	ErrInvalidCoordinates        = errors.New("target coordinates are out of range")
	ErrSameCoordinates           = errors.New("planet is already at those coordinates")
	ErrTargetOccupied            = errors.New("target coordinates are occupied or reserved")
	ErrInsufficientDarkMatter    = errors.New("insufficient dark matter")
	ErrBonusNotReady             = errors.New("the online bonus is not ready yet")
	ErrRelocationCooldown        = errors.New("planet was relocated too recently")
	ErrAttackLocked              = errors.New("planet cannot attack yet after relocation")
	ErrInsufficientUpgrades      = errors.New("insufficient upgrade drawings")
	ErrLotExpired                = errors.New("market lot has expired")
	ErrCannotBuyOwnLot           = errors.New("cannot buy your own market lot")
	ErrInsufficientAntimatter    = errors.New("insufficient antimatter")
	ErrInsufficientAchievement   = errors.New("insufficient achievement points")
	ErrGovernatorLevel           = errors.New("governator is already at that level or higher")
	ErrInsufficientStardust      = errors.New("insufficient stardust")
	ErrInsufficientAcademyPoints = errors.New("insufficient academy points")
	ErrAcademyPrereq             = errors.New("academy prerequisite not met")
	ErrAcademyLevel              = errors.New("academy skill is already at that level or higher")
	ErrNothingToReset            = errors.New("nothing to reset in this academy branch")
	ErrInsufficientDebris        = errors.New("insufficient debris field for the diameter expansion")
	ErrNoJumpgate                = errors.New("at least two moons with a Jumpgate are required")
	ErrJumpgateCooldown          = errors.New("the Jumpgate is still recharging")
	ErrPhalanxOffline            = errors.New("the Phalanx Sensor is offline after relocation")
	ErrPhalanxOutOfRange         = errors.New("target is out of Phalanx range")
	ErrPhalanxNoSensor           = errors.New("no Phalanx Sensor on this moon")
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
