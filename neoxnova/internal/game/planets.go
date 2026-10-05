package game

import (
	"math"
	"math/rand"
)

// MaxPlanetSlots is the number of colonisable positions per solar system. The
// slot after the last is the expedition (deep-space) slot.
const MaxPlanetSlots = 20

// slotRange is per-position min/max of a randomly rolled value.
type slotRange struct{ min, max int }

// The tables below are captured verbatim from niburuspace.com's galaxy
// colonisation tooltips (system 2:186, 20 slots). Fields peak mid-system; the
// outer slots are cold and near-worthless (see docs/COMBAT_SESSION_2026-10-05).
var (
	slotFields = [MaxPlanetSlots]slotRange{
		{362, 410}, {369, 417}, {373, 517}, {380, 565}, {399, 613},
		{465, 761}, {558, 787}, {558, 846}, {532, 1020}, {635, 920},
		{617, 898}, {606, 891}, {580, 839}, {558, 765}, {513, 643},
		{413, 458}, {310, 354}, {273, 317}, {236, 302}, {236, 284},
	}
	slotTempMin = [MaxPlanetSlots]slotRange{
		{220, 265}, {180, 225}, {140, 180}, {80, 120}, {65, 105},
		{40, 80}, {30, 60}, {15, 55}, {0, 40}, {-20, 20},
		{-40, 3}, {-45, -20}, {-50, -20}, {-70, -30}, {-100, -60},
		{-130, -90}, {-170, -130}, {-190, -160}, {-290, -240}, {-290, -240},
	}
	slotTempMax = [MaxPlanetSlots]slotRange{
		{260, 305}, {220, 265}, {180, 220}, {120, 160}, {105, 145},
		{80, 120}, {70, 100}, {55, 95}, {40, 80}, {20, 60},
		{0, 43}, {-5, 20}, {-10, 20}, {-30, 10}, {-60, -20},
		{-90, -50}, {-130, -90}, {-150, -120}, {-250, -200}, {-250, -200},
	}
)

// PlanetSpec is the rolled physical profile of a freshly colonised planet.
type PlanetSpec struct {
	FieldsMax int
	Diameter  int
	TempMin   int
	TempMax   int
}

// PlanetFieldsRange returns the colonisable field range for a 1-based slot.
func PlanetFieldsRange(slot int) (int, int, bool) {
	if slot < 1 || slot > MaxPlanetSlots {
		return 0, 0, false
	}
	r := slotFields[slot-1]
	return r.min, r.max, true
}

// RollPlanet deterministically rolls a planet's fields and temperature for a
// slot (uniform within the captured ranges; diameter is cosmetic).
func RollPlanet(seed int64, slot int) (PlanetSpec, bool) {
	if slot < 1 || slot > MaxPlanetSlots {
		return PlanetSpec{}, false
	}
	rng := rand.New(rand.NewSource(seed))
	roll := func(r slotRange) int { return r.min + rng.Intn(r.max-r.min+1) }
	fields := roll(slotFields[slot-1])
	tempMin := roll(slotTempMin[slot-1])
	tempMax := roll(slotTempMax[slot-1])
	if tempMax < tempMin {
		tempMin, tempMax = tempMax, tempMin
	}
	return PlanetSpec{
		FieldsMax: fields,
		Diameter:  int(math.Round(1000 * math.Sqrt(float64(fields)))),
		TempMin:   tempMin,
		TempMax:   tempMax,
	}, true
}

// SatelliteEnergy is the classic OGame solar-satellite output on niburu:
// round((maxTemp + 160) / 6). Verified against every captured slot tooltip.
func SatelliteEnergy(tempMax int) int {
	v := int(math.Round(float64(tempMax+160) / 6.0))
	if v < 0 {
		return 0
	}
	return v
}

// FieldSlotSeed is a stable per-coordinate seed so a colonisation roll is
// reproducible for the same position.
func FieldSlotSeed(galaxy, system, position int) int64 {
	return int64(galaxy)*1_000_000 + int64(system)*1_000 + int64(position)
}
