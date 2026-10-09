package game

import (
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// Expedition model (backlog item 1). Pure, deterministic-given-a-seed: the
// scheduler calls RollExpedition once when a fleet finishes its deep-space hold
// and persists whatever comes back. Live calibration: see
// docs/EXPEDITIONS_LIVE_2026-10-06.md (§2 outcome mix, §8 enemy formula).
//
// Enemy (items 1b/3): the defender is the sent fleet mirrored by a single
// per-fleet roll — pirates 0.60–0.69, aliens 0.85–0.94 — plus a small fixed
// template, and its Weapons/Shield/Armour is ONE rolled value scaling the
// attacker's general 109 bonus (pirates skew low ~0.1–1.8×, aliens sit at/above
// our bonus with a rare ~2.2× tail). Specific weapon techs / arsenal / academy are
// attacker-only — they are never mirrored (Combatant.FlatBonusPct carries the NPC
// roll). Combat reuses the pure engine in combat.go.
//
// PROVISIONAL: the outcome mix and per-outcome magnitudes are fitted to a few
// hundred live outcomes and are expected to be refined; the enemy mirror + roll
// are the measured part.

// ExpeditionOutcome is one of the possible results of an expedition.
type ExpeditionOutcome string

const (
	ExpeditionResources     ExpeditionOutcome = "resources"
	ExpeditionShips         ExpeditionOutcome = "ships"
	ExpeditionCombat        ExpeditionOutcome = "combat"
	ExpeditionDelay         ExpeditionOutcome = "delay"
	ExpeditionDarkMatter    ExpeditionOutcome = "darkmatter"
	ExpeditionFastReturn    ExpeditionOutcome = "fast"
	ExpeditionNothing       ExpeditionOutcome = "nothing"
	ExpeditionBlackHole     ExpeditionOutcome = "blackhole"
	ExpeditionBlackHoleLoot ExpeditionOutcome = "blackhole-loot"
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
	// Flavor selects the cosmetic message variant for the outcome. It is rolled
	// in RollExpedition so ExpeditionMessage stays pure/deterministic for a seed.
	Flavor int
}

// expeditionOutcomeWeights is the outcome distribution. Re-locked 2026-10-09
// against the fresh live harvest (docs/EXPEDITIONS… §11, n=772): owner kept
// combat 15 %, adopted the measured fatal black hole 1.68 % (was 0.5 %), added
// the rare POSITIVE black-hole loot (~1.7 %) and moved the freed weight into
// nothing/fast while preserving the live ships:resources:DM proportions
// (29.1:20.5:14.8). Sums to 1.0. PROVISIONAL beyond the locked rates.
var expeditionOutcomeWeights = []struct {
	outcome ExpeditionOutcome
	weight  float64
}{
	{ExpeditionResources, 0.2028},
	{ExpeditionShips, 0.2879},
	{ExpeditionCombat, 0.15},
	{ExpeditionNothing, 0.079},
	{ExpeditionDarkMatter, 0.1465},
	{ExpeditionDelay, 0.06},
	{ExpeditionFastReturn, 0.04},
	{ExpeditionBlackHoleLoot, 0.017},
	{ExpeditionBlackHole, 0.0168},
}

// npcWeights: within a combat encounter, live 58 pirates : 11 aliens = 84 / 16
// (docs/EXPEDITIONS… §11; was 70 / 30). Aliens stay much rarer.
var npcWeights = []struct {
	npc    NPCType
	weight float64
}{
	{NPCPirates, 0.84},
	{NPCAliens, 0.16},
}

// expeditionTemplate is the fixed minimum the enemy carries on top of the
// mirrored fleet (docs §6/§8/§11): tens–hundreds of ships the attacker often
// never sent. Small fleets are dominated by it, large fleets barely notice it.
// Bands widened 2026-10-09 for more randomness; 215/216 are alien-only (they
// only reach small fleets via the mirror otherwise).
var expeditionTemplate = []struct {
	code      string
	min, max  int64
	alienOnly bool
}{
	{"203", 20, 180, false}, // Heavy Cargo
	{"204", 5, 90, false},   // Light Fighter
	{"206", 5, 85, false},   // Cruiser
	{"207", 4, 90, false},   // Battleship
	{"213", 5, 45, false},   // Star Fighter
	{"215", 0, 20, true},    // Battlecruiser (aliens only)
	{"216", 0, 3, true},     // Black Moon (aliens only)
}

// expeditionCombatSalt decorrelates the battle RNG from the outcome RNG while
// keeping the whole result reproducible for a given seed.
const expeditionCombatSalt = int64(0x45585043) // "EXPC"

// Dark-matter finds scale with the sent fleet's points like resource finds
// (docs §11: live 1,553–7,044, median ≈ 3,642). The acc1 burn-down fleet is
// ~10k Frigates ⇒ FleetPoints ≈ 4.0e11, giving ~9.1e-9 DM/point at the median;
// the uniform factor below reproduces the observed window and 4.5× spread.
const (
	darkMatterPerPointLo = 3.3e-9
	darkMatterPerPointHi = 1.5e-8
	darkMatterFloor      = int64(100)
)

// expeditionFlavorVariants bounds the cosmetic Flavor index rolled per outcome.
const expeditionFlavorVariants = 16

// RollExpedition resolves one expedition for a fleet. `atk` must carry the fleet
// composition (atk.Units) plus the owner's combat techs/academy/upgrades; the
// same seed always yields the same result, so a retried scheduler event is
// idempotent. It never mutates its input.
func RollExpedition(atk Combatant, seed int64) ExpeditionResult {
	rng := rand.New(rand.NewSource(seed))
	fleet := atk.Units
	points := FleetPoints(fleet)

	res := ExpeditionResult{Outcome: rollWeightedOutcome(rng)}
	res.Flavor = rng.Intn(expeditionFlavorVariants)
	switch res.Outcome {
	case ExpeditionResources:
		res.Loot = expeditionLoot(FleetCargo(fleet), rng)

	case ExpeditionShips:
		res.Ships = expeditionRecovery(points, rng)
		res.UpgradeCode = rollExpeditionDrop(points, rng)

	case ExpeditionDarkMatter:
		res.DarkMatter = expeditionDarkMatter(points, rng)

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

	case ExpeditionBlackHoleLoot:
		// A rare POSITIVE black hole: resources are multiplied on the way through.
		res.Loot = expeditionBlackHoleLoot(FleetCargo(fleet), rng)

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

// expeditionDarkMatter scales a dark-matter find with the sent fleet's points
// (docs §11); fleets with no points keep the old flat 100–5000 range.
func expeditionDarkMatter(points int64, rng *rand.Rand) int64 {
	if points <= 0 {
		return 100 + rng.Int63n(4900)
	}
	perPoint := darkMatterPerPointLo + rng.Float64()*(darkMatterPerPointHi-darkMatterPerPointLo)
	dm := int64(math.Round(float64(points) * perPoint))
	if dm < darkMatterFloor {
		dm = darkMatterFloor
	}
	return dm
}

// expeditionBlackHoleLoot is the rare POSITIVE black-hole outcome: resources
// drawn into the anomaly are multiplied ("resources became much more"). It
// scales a normal resource find by 1.5–3× and still caps at the cargo hold.
func expeditionBlackHoleLoot(capacity int64, rng *rand.Rand) Cost {
	base := expeditionLoot(capacity, rng)
	total := base.Metal + base.Crystal + base.Deuterium
	if total <= 0 || capacity <= 0 {
		return Cost{}
	}
	total = int64(math.Round(float64(total) * (1.5 + rng.Float64()*1.5)))
	if total > capacity {
		total = capacity
	}
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

// ExpeditionMessage renders a resolved expedition as a player-facing message,
// mirroring the live sys_expe_* taxonomy (docs/EXPEDITIONS… §10/§11). It is pure
// so the engine can persist it verbatim and the API can serve it without the
// client re-deriving the flavour text.
func ExpeditionMessage(res ExpeditionResult) (title, body string) {
	switch res.Outcome {
	case ExpeditionResources:
		if res.Loot.Metal+res.Loot.Crystal+res.Loot.Deuterium == 0 {
			return "Expedition", "Our expedition found an asteroid belt, but the cargo holds were too small to carry anything home."
		}
		return "Expedition: resources found", "Our expedition discovered an abandoned supply depot and recovered " +
			formatLoot(res.Loot) + "."
	case ExpeditionShips:
		if len(res.Ships) == 0 {
			return "Expedition", "Our expedition found the wreck of a long-lost fleet, but nothing could be salvaged."
		}
		return "Expedition: ships found", "Our expedition came across the remains of an ancient battlefield and recovered " +
			formatShips(res.Ships) + "."
	case ExpeditionCombat:
		who := "pirates"
		if res.NPC == NPCAliens {
			who = "an alien species"
		}
		if res.Combat != nil && res.Combat.Winner == "attacker" {
			return "Expedition: battle won", "Our expedition was attacked by " + who + " and won the battle. Debris field: " +
				formatLoot(Cost{Metal: res.Combat.DebrisMetal, Crystal: res.Combat.DebrisCrystal}) + "."
		}
		return "Expedition: battle lost", "Our expedition encountered " + who + " and was destroyed in battle."
	case ExpeditionDarkMatter:
		return "Expedition: dark matter found", "Our expedition discovered an asteroid core containing dark matter."
	case ExpeditionDelay:
		return "Expedition: delayed", "Our expedition was delayed on its way home."
	case ExpeditionFastReturn:
		return "Expedition: early return", "Our expedition caught a favourable current and will return early."
	case ExpeditionBlackHole:
		return "Expedition: lost", "Our expedition was swallowed by a black hole. The fleet is lost."
	case ExpeditionBlackHoleLoot:
		return "Expedition: resources found", "Our ships were drawn into a black hole; some resources aboard became much more. Recovered: " +
			formatLoot(res.Loot) + "."
	default:
		return "Expedition", "Our expedition returned without any noteworthy findings."
	}
}

func formatLoot(c Cost) string {
	parts := make([]string, 0, 3)
	if c.Metal > 0 {
		parts = append(parts, strconv.FormatInt(c.Metal, 10)+" metal")
	}
	if c.Crystal > 0 {
		parts = append(parts, strconv.FormatInt(c.Crystal, 10)+" crystal")
	}
	if c.Deuterium > 0 {
		parts = append(parts, strconv.FormatInt(c.Deuterium, 10)+" deuterium")
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

func formatShips(ships map[string]int64) string {
	parts := make([]string, 0, len(ships))
	for code, n := range ships {
		if n <= 0 {
			continue
		}
		name := code
		if def, ok := LookupUnit(code); ok {
			name = def.Name
		}
		parts = append(parts, strconv.FormatInt(n, 10)+"x "+name)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
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
// bonus. The mirror and the strength roll are both enemy-type dependent
// (docs §11): pirates mirror less (0.60–0.69) and roll weaker overall, aliens
// mirror more (0.85–0.94) and roll at/above our bonus with a rare tail that can
// wipe the fleet.
func expeditionEnemy(fleet map[string]int64, atk Combatant, npc NPCType, rng *rand.Rand) (map[string]int64, float64) {
	var mirror, roll float64
	if npc == NPCAliens {
		mirror = 0.85 + rng.Float64()*0.09 // 0.85–0.94 (med ~0.90)
		roll = 0.70 + 1.80*math.Pow(rng.Float64(), 2)
	} else {
		mirror = 0.60 + rng.Float64()*0.09 // 0.60–0.69 (med ~0.64)
		roll = 0.10 + 1.70*math.Pow(rng.Float64(), 1.5)
	}
	enemy := map[string]int64{}
	for code, n := range fleet {
		if m := int64(math.Round(mirror * float64(n))); m > 0 {
			enemy[code] = m
		}
	}
	for _, t := range expeditionTemplate {
		if t.alienOnly && npc != NPCAliens {
			continue
		}
		cnt := t.min
		if t.max > t.min {
			cnt += rng.Int63n(t.max - t.min + 1)
		}
		if cnt > 0 {
			enemy[t.code] += cnt
		}
	}

	gen := MirroredResearchBonus(atk.Techs) // the mirrored 109/110/111 bonus
	return enemy, gen * roll
}
