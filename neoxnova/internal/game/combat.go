package game

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
)

// CombatSeed derives a deterministic battle seed from the stable inputs (unit
// counts, techs and academy skills) of both sides. The scheduler and any
// in-game simulator must use this same function, so that simulating an attack
// with identical fleets reproduces the real resolution exactly — the basis for
// trusting the battle simulator.
func CombatSeed(attacker, defender Combatant) int64 {
	h := fnv.New64a()
	write := func(c Combatant) {
		codes := make([]string, 0, len(c.Units))
		for k := range c.Units {
			codes = append(codes, k)
		}
		sort.Strings(codes)
		for _, k := range codes {
			fmt.Fprintf(h, "u%s=%d;", k, c.Units[k])
		}
		fmt.Fprintf(h, "t%d,%d,%d;", c.Techs.Weapons, c.Techs.Shield, c.Techs.Armour)
		skills := make([]string, 0, len(c.Academy))
		for k := range c.Academy {
			skills = append(skills, k)
		}
		sort.Strings(skills)
		for _, k := range skills {
			fmt.Fprintf(h, "a%s=%d;", k, c.Academy[k])
		}
		ups := make([]int, 0, len(c.Upgrades))
		for k := range c.Upgrades {
			ups = append(ups, k)
		}
		sort.Ints(ups)
		for _, k := range ups {
			fmt.Fprintf(h, "p%d=%.4f;", k, c.Upgrades[k])
		}
	}
	write(attacker)
	h.Write([]byte("|"))
	write(defender)
	return int64(h.Sum64())
}

// Combat engine for niburuspace.com (universe_6_niburu), reverse-engineered in
// docs/COMBAT_FINDINGS.md + docs/COMBAT_SESSION_2026-10-05.md. It is a pure,
// deterministic-given-a-seed round engine:
//   - up to MaxCombatRounds rounds (8); a "draw" is anything not decided in time
//   - every surviving unit fires once per round at ONE random live enemy unit
//   - shields absorb first and fully regenerate every round; hull persists
//   - overkill is discarded; there is NO per-shot bounce
//   - rapid fire grants extra shots with probability (rf-1)/rf (bounded)
//   - debris = 50% of base Metal+Crystal of destroyed SHIPS only (defenses 0)
//
// Reference-server quirks are deliberately NOT inherited (PHP divide-by-zero on
// attacker wins, defense-on-attacker-side malformed reports, etc.).

// MaxCombatRounds is the round cap observed on the reference server (draws end
// here for either side not being wiped).
const MaxCombatRounds = 8

// maxRapidFireShots bounds the rapid-fire chain so a pathological RF table can
// never spin forever.
const maxRapidFireShots = 10000

// Techs are the three empire-wide combat technologies (codes 109/110/111).
type CombatTechs struct {
	Weapons int
	Shield  int
	Armour  int
}

// Combatant is one side of a battle: unit counts plus its combat modifiers.
// AcademyDamagePct is a flat additive attack bonus (the reference server's
// attacker-side academy procs are real but not yet modelled per-skill; 0 = off).
type Combatant struct {
	Units   map[string]int64
	Techs   CombatTechs
	Academy map[string]int // skill code -> level (1103 Double attack, 1109 Chain reaction, ...)
	// Upgrades maps an Arsenal upgrade code (1..19) to the account's accumulated
	// bonus percent (account_upgrades.value). Each unit applies the bonus for its
	// card class, folded additively into the tech bonus.
	Upgrades map[int]float64
	// AcademyDamagePct is a legacy flat attack bonus (kept for callers that only
	// have an aggregate figure). Prefer Academy.
	AcademyDamagePct float64
}

// upgradePct returns the accumulated Arsenal bonus (in percent) for a code.
func (c Combatant) upgradePct(code int) float64 {
	if code == 0 || len(c.Upgrades) == 0 {
		return 0
	}
	return c.Upgrades[code]
}

// SideReport is the per-unit outcome for one side.
type SideReport struct {
	Initial   map[string]int64
	Lost      map[string]int64
	Remaining map[string]int64
}

// RoundLoss is the per-unit losses suffered by both sides during one round.
type RoundLoss struct {
	Attacker map[string]int64
	Defender map[string]int64
}

// CombatResult is the outcome of Resolve.
type CombatResult struct {
	Winner        string // "attacker", "defender" or "draw"
	Rounds        int
	Attacker      SideReport
	Defender      SideReport
	RoundLosses   []RoundLoss // one entry per executed round
	DebrisMetal   int64
	DebrisCrystal int64
	MoonChance    int // percent, 0..20 (reference server caps at 20)
}

func lossDelta(prev, cur map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for code, v := range cur {
		if d := v - prev[code]; d > 0 {
			out[code] = d
		}
	}
	return out
}

func cloneCounts(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// TechBonus returns the server-custom quadratic tech bonus in percent:
// round(L*(L+2)/4). L1=1, L5=9, L10=30, L15=64, L20=110.
func TechBonus(level int) int {
	if level < 0 {
		level = 0
	}
	return int(math.Round(float64(level*(level+2)) / 4.0))
}

// DerivedStat applies a tech bonus to a base stat: round(base*(1+bonus/100)).
func DerivedStat(base, techLevel int) int {
	return int(math.Round(float64(base) * (1.0 + float64(TechBonus(techLevel))/100.0)))
}

// DerivedStatBonus is DerivedStat with an extra additive percentage (the
// Arsenal upgrade bonus for the unit's class). Both bonuses share one percent
// sum so they never compound: round(base*(1+(tech+extra)/100)).
func DerivedStatBonus(base, techLevel int, extraPct float64) int {
	return int(math.Round(float64(base) * (1.0 + (float64(TechBonus(techLevel))+extraPct)/100.0)))
}

// MoonChance is the classic OGame formula: min(20, (M+C)/100000) percent. The
// reference server reports 0% and never spawned a moon, but the target game
// adopts the standard formula (see COMBAT_SESSION_2026-10-05 §3).
func MoonChance(debrisMetal, debrisCrystal int64) int {
	total := debrisMetal + debrisCrystal
	if total < 0 {
		return 0
	}
	pct := int(total / 100000)
	if pct > 20 {
		pct = 20
	}
	return pct
}

// Loot returns the resources captured from a defender, in M->C->D order: 50% of
// each stored resource, capped by the surviving cargo capacity.
func Loot(stored Cost, cargoCapacity int64) Cost {
	remaining := cargoCapacity
	take := func(v int64) int64 {
		want := v / 2
		if want > remaining {
			want = remaining
		}
		if want < 0 {
			want = 0
		}
		remaining -= want
		return want
	}
	return Cost{Metal: take(stored.Metal), Crystal: take(stored.Crystal), Deuterium: take(stored.Deuterium)}
}

// cargoCapacity holds the server-scaled cargo holds (owner-confirmed). Light and
// heavy cargo are only meatshields at these rates; Battle Transporter/Recycler
// are the real freighters.
var cargoCapacity = map[string]int64{
	"202": 25000,
	"203": 50000,
	"209": 20000,
	"217": 400000000,
	"219": 200000000,
}

// CargoCapacity returns the cargo hold of a unit (0 if unknown).
func CargoCapacity(code string) int64 { return cargoCapacity[code] }

// FleetCargo sums the cargo capacity of a fleet.
func FleetCargo(units map[string]int64) int64 {
	var total int64
	for code, n := range units {
		if n > 0 {
			total += CargoCapacity(code) * n
		}
	}
	return total
}

type combatType struct {
	code    string
	attack  int
	shield  int
	hull    int
	isShip  bool
	debrisM int64
	debrisC int64
}

type combatInstance struct {
	def    *combatType
	hull   int
	shield int
}

type combatSide struct {
	instances []*combatInstance
}

func (s *combatSide) removeAt(i int) {
	last := len(s.instances) - 1
	s.instances[i] = s.instances[last]
	s.instances = s.instances[:last]
}

func isShipCode(code string) bool { return len(code) == 3 && code[0] == '2' }

func buildSide(c Combatant) (*combatSide, SideReport) {
	side := &combatSide{}
	report := SideReport{Initial: map[string]int64{}, Lost: map[string]int64{}, Remaining: map[string]int64{}}
	// Iterate codes in a stable order so a given seed always produces the same
	// battle regardless of Go's randomised map iteration.
	codes := make([]string, 0, len(c.Units))
	for code := range c.Units {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		count := c.Units[code]
		if count <= 0 {
			continue
		}
		stats, ok := unitStats[code]
		if !ok {
			continue // unknown unit: cannot model
		}
		classes := unitClasses[code]
		ct := &combatType{
			code:   code,
			attack: DerivedStatBonus(stats.Attack, c.Techs.Weapons, c.upgradePct(UpgradeCodeForClass("weapon", classes.Weapon))),
			shield: DerivedStatBonus(stats.Shield, c.Techs.Shield, c.upgradePct(UpgradeCodeForClass("shield", classes.Shield))),
			hull:   DerivedStatBonus(stats.Hull, c.Techs.Armour, c.upgradePct(UpgradeCodeForClass("armor", classes.Armor))),
			isShip: isShipCode(code),
		}
		if c.AcademyDamagePct != 0 {
			ct.attack = int(math.Round(float64(ct.attack) * (1.0 + c.AcademyDamagePct/100.0)))
		}
		if def, ok := LookupUnit(code); ok {
			ct.debrisM = def.BaseCost.Metal / 2
			ct.debrisC = def.BaseCost.Crystal / 2
		}
		report.Initial[code] = count
		for i := int64(0); i < count; i++ {
			side.instances = append(side.instances, &combatInstance{def: ct, hull: ct.hull, shield: ct.shield})
		}
	}
	return side, report
}

func resetShields(s *combatSide) {
	for _, u := range s.instances {
		u.shield = u.def.shield
	}
}

func applyDamage(dmg int, t *combatInstance) {
	if dmg <= 0 {
		return
	}
	if t.shield > 0 {
		absorbed := dmg
		if absorbed > t.shield {
			absorbed = t.shield
		}
		t.shield -= absorbed
		dmg -= absorbed
	}
	if dmg > 0 {
		t.hull -= dmg
	}
}

// sideProcs holds the academy proc percentages that are actually applied in the
// round engine. The reference server's attacker-side procs are real (confirmed
// 2026-10-05), but only these two have a defensible interpretation from the
// captured skill texts; they are expressed as per-shot probabilities:
//
//	1103 "Double attack"  (+2%/lvl cumulative chance of an extra shot shell)
//	1109 "Chain reaction" (+1%/lvl chance to fire again after a kill)
//
// The remaining procs (1108/1110/1111, and the defence branch 1303/1308/1311)
// are not yet calibrated and are intentionally left at zero.
type sideProcs struct {
	doubleShot int // 1103, percent
	chain      int // 1109, percent
}

func procsFor(academy map[string]int) sideProcs {
	if len(academy) == 0 {
		return sideProcs{}
	}
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}
	return sideProcs{
		doubleShot: clamp(2 * academy["1103"]),
		chain:      clamp(academy["1109"]),
	}
}

// round1ShotProbability is the reference server's per-shot round-1 factor: a
// unit fires its full rapid-fire quota from round 2 onward, but each round-1
// shot only lands with this probability (observed ~0.70). This reproduces e.g.
// a Battleship (RF 2 vs LF) killing ~1.4 LF in round 1 and exactly 2/round after.
const round1ShotProbability = 0.70

// fireVolley has every unit in shooters fire (with rapid fire) at targets.
func fireVolley(shooters, targets *combatSide, rng *rand.Rand, lost map[string]int64, res *CombatResult, pr sideProcs, round1 bool) {
	for _, s := range shooters.instances {
		if len(targets.instances) == 0 {
			return
		}
		shoot(s, targets, rng, lost, res, pr, round1)
	}
}

// shoot fires one unit: a base shot, then a deterministic rapid-fire quota equal
// to the RF value against the first target hit (steady from round 2). Academy
// procs may add shots. In round 1 each shot lands only with
// round1ShotProbability.
func shoot(s *combatInstance, targets *combatSide, rng *rand.Rand, lost map[string]int64, res *CombatResult, pr sideProcs, round1 bool) {
	shots := 1
	for i := 0; ; i++ {
		if i >= shots {
			// Academy 1103 Double attack: one bonus shot.
			if i == shots && pr.doubleShot > 0 && rng.Intn(100) < pr.doubleShot {
				shots++
			} else {
				return
			}
		}
		if i >= maxRapidFireShots || len(targets.instances) == 0 {
			return
		}
		if round1 && rng.Float64() >= round1ShotProbability {
			continue // shot skipped this round
		}
		t, killed := hit(s, targets, rng, lost, res)
		if i == 0 {
			if rf := rapidFire[s.def.code][t.def.code]; rf > shots {
				shots = rf
			}
		}
		// Academy 1109 Chain reaction: an extra shot after a kill.
		if pr.chain > 0 && killed && rng.Intn(100) < pr.chain {
			shots++
		}
	}
}

// hit resolves one shot: pick a random live target, damage it, and on a kill
// record the loss + debris and remove it from the live pool. It returns the
// (possibly killed) target and whether it died.
func hit(s *combatInstance, targets *combatSide, rng *rand.Rand, lost map[string]int64, res *CombatResult) (*combatInstance, bool) {
	i := rng.Intn(len(targets.instances))
	t := targets.instances[i]
	applyDamage(s.def.attack, t)
	killed := t.hull <= 0
	if killed {
		lost[t.def.code]++
		if t.def.isShip {
			res.DebrisMetal += t.def.debrisM
			res.DebrisCrystal += t.def.debrisC
		}
		targets.removeAt(i)
	}
	return t, killed
}

// Resolve runs one battle to completion. The same seed always yields the same
// result (useful for replay tests); pass a nonzero seed for live resolution.
func Resolve(attacker, defender Combatant, seed int64) CombatResult {
	rng := rand.New(rand.NewSource(seed))
	a, aRep := buildSide(attacker)
	d, dRep := buildSide(defender)
	aProcs, dProcs := procsFor(attacker.Academy), procsFor(defender.Academy)
	res := CombatResult{Attacker: aRep, Defender: dRep}
	prevA, prevD := map[string]int64{}, map[string]int64{}

	for res.Rounds < MaxCombatRounds {
		if len(a.instances) == 0 || len(d.instances) == 0 {
			break
		}
		res.Rounds++
		resetShields(a)
		resetShields(d)
		round1 := res.Rounds == 1
		fireVolley(a, d, rng, res.Defender.Lost, &res, aProcs, round1)
		fireVolley(d, a, rng, res.Attacker.Lost, &res, dProcs, round1)
		res.RoundLosses = append(res.RoundLosses, RoundLoss{
			Attacker: lossDelta(prevA, res.Attacker.Lost),
			Defender: lossDelta(prevD, res.Defender.Lost),
		})
		prevA, prevD = cloneCounts(res.Attacker.Lost), cloneCounts(res.Defender.Lost)
	}

	switch {
	case len(a.instances) == 0 && len(d.instances) == 0:
		res.Winner = "draw"
	case len(d.instances) == 0:
		res.Winner = "attacker"
	case len(a.instances) == 0:
		res.Winner = "defender"
	default:
		res.Winner = "draw"
	}

	fillRemaining(res.Attacker)
	fillRemaining(res.Defender)
	res.MoonChance = MoonChance(res.DebrisMetal, res.DebrisCrystal)
	return res
}

func fillRemaining(r SideReport) {
	for code, initial := range r.Initial {
		remaining := initial - r.Lost[code]
		if remaining < 0 {
			remaining = 0
		}
		r.Remaining[code] = remaining
	}
}
