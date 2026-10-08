package game

import (
	"math"
	"math/rand"
)

// Expedition model (backlog item 1). Pure, deterministic-given-a-seed: the
// scheduler calls RollExpedition once when a fleet finishes its deep-space hold
// and persists whatever comes back. Live calibration: see
// docs/EXPEDITIONS_LIVE_2026-10-06.md (§2 outcome mix, §8 enemy formula).
//
// Enemy (item 1b): the defender is the sent fleet mirrored by a single per-fleet
// roll U(0.60..0.90) plus a small fixed template, and its Weapons/Shield/Armour
// is ONE rolled value scaling the attacker's general 109 bonus (Pirates ~0.1–1.7×,
// Aliens up to ~2.2×). Specific weapon techs / arsenal / academy are
// attacker-only — they are never mirrored (Combatant.FlatBonusPct carries the NPC
// roll). Combat reuses the pure engine in combat.go.
//
// PROVISIONAL: the outcome mix and per-outcome magnitudes are fitted to a few
// hundred live outcomes and are expected to be refined; the enemy mirror + roll
// are the measured part.

// ExpeditionOutcome is one of the possible results of an expedition.
type ExpeditionOutcome string

const (
	ExpeditionResources  ExpeditionOutcome = "resources"
	ExpeditionShips      ExpeditionOutcome = "ships"
	ExpeditionCombat     ExpeditionOutcome = "combat"
	ExpeditionDelay      ExpeditionOutcome = "delay"
	ExpeditionDarkMatter ExpeditionOutcome = "darkmatter"
	ExpeditionFastReturn ExpeditionOutcome = "fast"
	ExpeditionNothing    ExpeditionOutcome = "nothing"
	ExpeditionBlackHole  ExpeditionOutcome = "blackhole"
)

// NPCType is the faction an expedition fights. Only Pirates and Aliens exist
// (owner-confirmed 2026-10-08); the "barbarians" wording in the live capture is
// just a pirate flavour string.
type NPCType string

const (
	NPCPirates NPCType = "pirates"
	NPCAliens  NPCType = "aliens"
)

// ExpeditionResult is the pure outcome of one expedition. The engine maps it to
// persistence: cargo, recovered ships, an optional battle already resolved by the
// combat engine, an optional Arsenal drawing and a return-time adjustment.
type ExpeditionResult struct {
	Outcome ExpeditionOutcome

	// Combat payload (Outcome == ExpeditionCombat).
	NPC               NPCType
	EnemyUnits        map[string]int64
	EnemyFlatBonusPct float64
	Combat            *CombatResult

	// Resources found (Outcome == ExpeditionResources), already capped by the
	// fleet's cargo hold.
	Loot Cost
	// DarkMatter found (Outcome == ExpeditionDarkMatter).
	DarkMatter int64
	// Recovered ships added to the returning fleet (Outcome == ExpeditionShips).
	Ships map[string]int64
	// ReturnAdjustSecs shifts the return ETA: positive = delayed, negative =
	// early (Outcome == ExpeditionDelay / ExpeditionFastReturn).
	ReturnAdjustSecs int64
	// UpgradeCode is an Arsenal drawing (1..19) found on a win / ship find, or 0.
	UpgradeCode int
}

// expeditionOutcomeWeights is the outcome distribution. It tracks the open
// (OGame/2Moons/XNova) default mix — resources 32.5 %, ships 22 %, dark matter
// 9 %, combat 8.4 % (pirates 5.8 / aliens 2.6), delay 7 %, early return 2 %,
// nothing 18.6 %, black hole 0.33 % — rescaled here so combat is the live ~15 %
// and the black hole the measured ~2 %. Everyone else is scaled down
// proportionally. PROVISIONAL.
var expeditionOutcomeWeights = []struct {
	outcome ExpeditionOutcome
	weight  float64
}{
	{ExpeditionResources, 0.30},
	{ExpeditionShips, 0.20},
	{ExpeditionCombat, 0.15},
	{ExpeditionNothing, 0.17},
	{ExpeditionDarkMatter, 0.08},
	{ExpeditionDelay, 0.06},
	{ExpeditionFastReturn, 0.02},
	{ExpeditionBlackHole, 0.02},
}

// npcWeights: within a combat encounter the open-codebase split is 5.8 % pirates
// vs 2.6 % aliens ⇒ 70 / 30 (owner: Aliens must stay much rarer).
var npcWeights = []struct {
	npc    NPCType
	weight float64
}{
	{NPCPirates, 0.70},
	{NPCAliens, 0.30},
}

// expeditionTemplate is the fixed minimum the enemy carries on top of the
// mirrored fleet (docs §6/§8): tens–hundreds of ships the attacker often never
// sent. Small fleets are dominated by it, large fleets barely notice it.
var expeditionTemplate = []struct {
	code     string
	min, max int64
}{
	{"203", 40, 120}, // Heavy Cargo
	{"204", 10, 40},  // Light Fighter
	{"206", 20, 60},  // Cruiser
	{"207", 4, 15},   // Battleship
	{"213", 8, 20},   // Star Fighter
}

// expeditionCombatSalt decorrelates the battle RNG from the outcome RNG while
// keeping the whole result reproducible for a given seed.
const expeditionCombatSalt = int64(0x45585043) // "EXPC"

// RollExpedition resolves one expedition for a fleet. `atk` must carry the fleet
// composition (atk.Units) plus the owner's combat techs/academy/upgrades; the
// same seed always yields the same result, so a retried scheduler event is
// idempotent. It never mutates its input.
func RollExpedition(atk Combatant, seed int64) ExpeditionResult {
	rng := rand.New(rand.NewSource(seed))
	fleet := atk.Units
	points := FleetPoints(fleet)

	res := ExpeditionResult{Outcome: rollWeightedOutcome(rng)}
	switch res.Outcome {
	case ExpeditionResources:
		res.Loot = expeditionLoot(FleetCargo(fleet), rng)

	case ExpeditionShips:
		res.Ships = expeditionRecovery(points, rng)
		res.UpgradeCode = rollExpeditionDrop(points, rng)

	case ExpeditionDarkMatter:
		res.DarkMatter = 100 + rng.Int63n(4900)

	case ExpeditionDelay:
		res.ReturnAdjustSecs = 600 + rng.Int63n(6600) // +10 min .. ~+2 h

	case ExpeditionFastReturn:
		res.ReturnAdjustSecs = -(600 + rng.Int63n(5400)) // -10 min .. -1.5 h

	case ExpeditionCombat:
		res.NPC = rollNPC(rng)
		res.EnemyUnits, res.EnemyFlatBonusPct = expeditionEnemy(fleet, atk, res.NPC, rng)
		def := Combatant{Units: res.EnemyUnits, FlatBonusPct: res.EnemyFlatBonusPct}
		battle := Resolve(atk, def, seed^expeditionCombatSalt)
		res.Combat = &battle
		// Arsenal draws drop from won fights (~10 %, tier by fleet points).
		if battle.Winner == "attacker" {
			res.UpgradeCode = rollExpeditionDrop(points, rng)
		}

	case ExpeditionBlackHole:
		// The engine wipes the fleet; nothing else to compute.
	}
	return res
}

// FleetPoints is the metal+crystal value of a fleet's base hulls. It is the
// expedition scale input and the Arsenal drop tier driver.
func FleetPoints(units map[string]int64) int64 {
	var total int64
	for code, n := range units {
		if n <= 0 {
			continue
		}
		if def, ok := LookupUnit(code); ok {
			total += (def.BaseCost.Metal + def.BaseCost.Crystal) * n
		}
	}
	return total
}

// rollWeightedOutcome picks an outcome by cumulative weight.
func rollWeightedOutcome(rng *rand.Rand) ExpeditionOutcome {
	var total float64
	for _, w := range expeditionOutcomeWeights {
		total += w.weight
	}
	r := rng.Float64() * total
	for _, w := range expeditionOutcomeWeights {
		if r < w.weight {
			return w.outcome
		}
		r -= w.weight
	}
	return expeditionOutcomeWeights[len(expeditionOutcomeWeights)-1].outcome
}

// rollNPC picks the combat faction by weight.
func rollNPC(rng *rand.Rand) NPCType {
	var total float64
	for _, w := range npcWeights {
		total += w.weight
	}
	r := rng.Float64() * total
	for _, w := range npcWeights {
		if r < w.weight {
			return w.npc
		}
		r -= w.weight
	}
	return npcWeights[len(npcWeights)-1].npc
}

// expeditionLoot is a resource find capped by the fleet's cargo hold: a random
// 30–100 % of capacity split metal/crystal/deuterium (live returns are
// metal-heavy). Returns zero for a fleet with no cargo capacity.
func expeditionLoot(capacity int64, rng *rand.Rand) Cost {
	if capacity <= 0 {
		return Cost{}
	}
	total := int64(float64(capacity) * (0.30 + rng.Float64()*0.70))
	metal := int64(float64(total) * 0.55)
	crystal := int64(float64(total) * 0.35)
	deut := total - metal - crystal
	if deut < 0 {
		deut = 0
	}
	return Cost{Metal: metal, Crystal: crystal, Deuterium: deut}
}

// expeditionRecovery generates a "deserted base / predecessor wreck" find worth
// 5–35 % of the sent fleet's points, across 1–3 random ship types (live finds
// were large mixed batches, e.g. LC 787k + HC 22k + HF 1.7k + Cruiser 262).
func expeditionRecovery(fleetPoints int64, rng *rand.Rand) map[string]int64 {
	if fleetPoints <= 0 {
		return nil
	}
	budget := int64(float64(fleetPoints) * (0.05 + rng.Float64()*0.30))
	if budget <= 0 {
		return nil
	}
	types := []string{"202", "203", "204", "205", "206", "207", "213", "215", "217", "219", "225"}
	out := map[string]int64{}
	n := 1 + rng.Intn(3)
	remaining := budget
	for i := 0; i < n && remaining > 0; i++ {
		code := types[rng.Intn(len(types))]
		def, ok := LookupUnit(code)
		if !ok {
			continue
		}
		pts := def.BaseCost.Metal + def.BaseCost.Crystal
		if pts <= 0 {
			continue
		}
		share := remaining
		if i < n-1 {
			share = remaining/2 + rng.Int63n(remaining/2+1)
		}
		count := share / pts
		if count < 1 {
			count = 1
		}
		out[code] += count
		remaining -= count * pts
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// rollExpeditionDrop rolls one Arsenal drawing for an expedition win / ship find.
// The tier comes from the sent fleet's points; the chance is the flat catalog
// value. Returns 0 when nothing drops.
func rollExpeditionDrop(fleetPoints int64, rng *rand.Rand) int {
	code, ok := RollDrop(ArsenalTier(fleetPoints), rng.Float64(), rng.Float64())
	if !ok {
		return 0
	}
	return code
}

// MirroredResearchBonus is the general Weapons/Shield/Armour research
// (codes 109/110/111) that an expedition NPC mirrors. The NPC applies ONE rolled
// value to all three stats, so the base is the strongest of the three general
// bonuses. Specific weapon techs (120/121/122/199), Arsenal upgrades, Academy
// skills and governors are NEVER mirrored — real combat only.
func MirroredResearchBonus(t CombatTechs) float64 {
	return math.Max(
		float64(TechBonus(t.Weapons)),
		math.Max(float64(TechBonus(t.Shield)), float64(TechBonus(t.Armour))),
	)
}

// expeditionEnemy builds the mirrored defender plus its single rolled W/S/A
// bonus. Aliens roll higher than pirates (the fleet wipes).
func expeditionEnemy(fleet map[string]int64, atk Combatant, npc NPCType, rng *rand.Rand) (map[string]int64, float64) {
	factor := 0.60 + rng.Float64()*0.30 // one roll per fleet, median ~0.66
	enemy := map[string]int64{}
	for code, n := range fleet {
		if m := int64(math.Round(factor * float64(n))); m > 0 {
			enemy[code] = m
		}
	}
	for _, t := range expeditionTemplate {
		cnt := t.min
		if t.max > t.min {
			cnt += rng.Int63n(t.max - t.min + 1)
		}
		if cnt > 0 {
			enemy[t.code] += cnt
		}
	}

	gen := MirroredResearchBonus(atk.Techs) // the mirrored 109/110/111 bonus
	var roll float64
	if npc == NPCAliens {
		roll = 0.5 + rng.Float64()*1.9 // ~0.5–2.4× (median ~1.0, wipes at ~2.2×)
	} else {
		roll = 0.1 + rng.Float64()*1.7 // pirates: ~0.1–1.8×
	}
	return enemy, gen * roll
}
