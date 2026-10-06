package game

// Coordinate domain limits, mirroring the coord_galaxy/coord_system domains in
// migrations/0001_init.sql.
const (
	MaxGalaxies = 9
	MaxSystems  = 499
)

// Planet relocation ("teleport") pricing, captured verbatim from the
// niburuspace.com Planetarium (Black market -> Planetarium) on 2026-10-06
// (planet 3:125:12, docs/screenshots_niburu/). The fee is exactly linear and
// decomposes by coordinate axis:
//
//	cost = 15000*|dGalaxy| + 1000*|dSystem| + 2500*|dPosition|
//
// Captured samples: 3:125:12->3:125:13 = 2500, ->3:126:12 = 1000,
// ->3:127:14 = 7000, ->2:125:12 = 15000, ->2:127:14 = 22000.
const (
	// RelocationDMperGalaxy is the Dark Matter fee per galaxy of distance.
	RelocationDMperGalaxy int64 = 15000
	// RelocationDMperSystem is the Dark Matter fee per system of distance.
	RelocationDMperSystem int64 = 1000
	// RelocationDMperPosition is the Dark Matter fee per planetary slot.
	RelocationDMperPosition int64 = 2500
)

// ValidRelocationTarget reports whether a destination lies within the universe's
// planet domain: galaxy 1..MaxGalaxies, system 1..MaxSystems and a colonisable
// position 1..planetsPerSystem (position 21 is the expedition slot and never a
// planet).
func ValidRelocationTarget(galaxy, system, position, planetsPerSystem int) bool {
	return galaxy >= 1 && galaxy <= MaxGalaxies &&
		system >= 1 && system <= MaxSystems &&
		position >= 1 && position <= planetsPerSystem
}

// RelocationCost returns the Dark Matter fee to teleport a planet between two
// coordinate vectors. The reference price is exactly linear per axis (see the
// constants above); relocating to the same coordinates is free.
func RelocationCost(fromGalaxy, fromSystem, fromPosition, toGalaxy, toSystem, toPosition int) int64 {
	return RelocationDMperGalaxy*absDiff(fromGalaxy, toGalaxy) +
		RelocationDMperSystem*absDiff(fromSystem, toSystem) +
		RelocationDMperPosition*absDiff(fromPosition, toPosition)
}

// RelocationLeavesSystem reports whether a teleport changes the solar system or
// galaxy. The reference server allows unlimited same-system teleports but limits
// inter-system/galaxy teleports to one per hour.
func RelocationLeavesSystem(fromGalaxy, fromSystem, toGalaxy, toSystem int) bool {
	return fromGalaxy != toGalaxy || fromSystem != toSystem
}

func absDiff(a, b int) int64 {
	if a > b {
		return int64(a - b)
	}
	return int64(b - a)
}
