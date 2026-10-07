package game

import "math"

// Moon formulas (standard OGame), verified against the live acc1 moon
// (created at 20 % with diameter 8,426 km — the exact value the diameter
// formula yields for x=11, p=20). See docs/MOONS.md.
//
// The creation chance itself lives in combat.go: MoonChance(metal, crystal) =
// min(floor((metal+crystal)/100000), 20) percent.

// MoonIndestructibleKm is the server rule: a moon at or above this diameter
// cannot be destroyed. Standard moons cap out at 8,944 km, so this is a
// server-specific safeguard.
const MoonIndestructibleKm = 10000

// MoonDiameterKm is the standard OGame moon diameter:
//
//	floor( sqrt(x + 3*p) * 1000 ) km
//
// where p is the creation chance (percent) and x is a uniform integer in
// [10,20]. x=10..20 yields 3,605 km (p=1) up to 8,944 km (p=20); at the 20 %
// cap a moon is always >= 8,366 km. roll is clamped into [10,20].
func MoonDiameterKm(chancePct, roll int) int {
	if chancePct < 1 {
		chancePct = 1
	}
	if roll < 10 {
		roll = 10
	}
	if roll > 20 {
		roll = 20
	}
	return int(math.Floor(math.Sqrt(float64(roll+3*chancePct)) * 1000.0))
}

// MoonFields is the classic field count from a moon's diameter:
// floor((diameter/1000)^2). NOTE: this server instead starts a moon at 0 fields
// and grants fields via the Moon base (41) — kept for reference/reconciliation.
func MoonFields(diameterKm int) int {
	if diameterKm <= 0 {
		return 0
	}
	f := float64(diameterKm) / 1000.0
	return int(f * f)
}

// MoonDestruction returns the percent chance to destroy a moon of diameterKm
// using `deathstars` Battle Fortresses, and the percent chance the attacking
// Battle Fortresses are destroyed (a single roll for the whole fleet):
//
//	destroy  = (100 - sqrt(S)) * sqrt(D)
//	rip loss = sqrt(S) / 2
//
// A moon at/above MoonIndestructibleKm can never be destroyed. PROVISIONAL:
// standard OGame, not yet measured on this server.
func MoonDestruction(diameterKm, deathstars int) (destroyPct, deathstarLossPct float64) {
	if diameterKm <= 0 || deathstars <= 0 || diameterKm >= MoonIndestructibleKm {
		return 0, 0
	}
	root := math.Sqrt(float64(diameterKm))
	destroyPct = (100.0 - root) * math.Sqrt(float64(deathstars))
	if destroyPct < 0 {
		destroyPct = 0
	}
	if destroyPct > 100 {
		destroyPct = 100
	}
	deathstarLossPct = root / 2.0
	return destroyPct, deathstarLossPct
}
