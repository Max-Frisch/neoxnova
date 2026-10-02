package utils

import (
	"math"
)

// Global Universe Configuration Multipliers from your Niburu Space logs
const (
	UniverseFleetSpeed = 15.0 // The 15x Fleet Speed Multiplier
)

// CalculateCoordinateDistance finds the absolute spatial distance between two planet vectors
func CalculateCoordinateDistance(g1, s1, p1, g2, s2, p2 int) float64 {
	if g1 != g2 {
		return 20000.0 * math.Abs(float64(g1-g2))
	}
	if s1 != s2 {
		return 2700.0 + 95.0*math.Abs(float64(s1-s2))
	}
	return 1000.0 + 5.0*math.Abs(float64(p1-p2))
}

// CalculateFlightDuration calculates the exact flight trip time in seconds
func CalculateFlightDuration(distance float64, baseMaxSpeed int, speedPercent int) int64 {
	// speedPercent is the user velocity toggle throttle (10 to 100)
	velocityModifier := float64(speedPercent) / 100.0
	
	// Traditional OGame/2Moons flight duration formula mapping
	rawDuration := math.Round(35000.0/velocityModifier*math.Sqrt(distance*10.0/float64(baseMaxSpeed))) + 10.0
	
	// Accelerate the time steps using the server's native high-rate speed multiplier scale
	return int64(math.Max(1, math.Round(rawDuration/UniverseFleetSpeed)))
}

// CalculateDeuteriumConsumption returns the exact fuel burn cost for the mission
func CalculateDeuteriumConsumption(shipCount int64, baseFuelConsumption int, distance float64) int64 {
	// Base scalar engine combustion propulsion burn calculation
	fuelBurn := float64(shipCount) * float64(baseFuelConsumption) * (distance / 35000.0)
	return int64(math.Max(1, math.Round(fuelBurn+1.0)))
}
