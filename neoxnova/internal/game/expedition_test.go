package game

import (
	"math/rand"
	"sort"
	"testing"
)

func expSampleAtk() Combatant {
	return Combatant{
		Units:   map[string]int64{"227": 500, "219": 25, "202": 1, "207": 1},
		Techs:   CombatTechs{Weapons: 16, Shield: 16, Armour: 17, Laser: 20, Ion: 18, Plasma: 15},
		Academy: map[string]int{"1103": 1},
	}
}

// TestRollExpeditionDeterministic: the same seed must reproduce the whole result
// (including the resolved battle), so a retried scheduler event is idempotent.
func TestRollExpeditionDeterministic(t *testing.T) {
	atk := expSampleAtk()
	a := RollExpedition(atk, 12345)
	b := RollExpedition(atk, 12345)
	if a.Outcome != b.Outcome {
		t.Fatalf("outcome drift: %s vs %s", a.Outcome, b.Outcome)
	}
	if (a.Combat == nil) != (b.Combat == nil) {
		t.Fatalf("combat presence drift")
	}
	if a.Combat != nil && a.Combat.Winner != b.Combat.Winner {
		t.Fatalf("combat winner drift: %s vs %s", a.Combat.Winner, b.Combat.Winner)
	}
	if a.UpgradeCode != b.UpgradeCode || a.ReturnAdjustSecs != b.ReturnAdjustSecs {
		t.Fatalf("scalar drift: %+v vs %+v", a, b)
	}
	// A different seed should (very likely) differ somewhere.
	if c := RollExpedition(atk, 999); c.Outcome == a.Outcome && c.UpgradeCode == a.UpgradeCode &&
		c.ReturnAdjustSecs == a.ReturnAdjustSecs && len(c.Ships) == len(a.Ships) && c.DarkMatter == a.DarkMatter {
		t.Log("note: seeds 12345 and 999 produced identical scalar outcomes (allowed)")
	}
}

// TestExpeditionOutcomeMix checks every outcome is reachable and the locked
// rates hold: combat 15 %, fatal black hole 1.68 %, positive BH-loot 1.7 %.
func TestExpeditionOutcomeMix(t *testing.T) {
	atk := expSampleAtk()
	const n = 40000
	counts := map[ExpeditionOutcome]int{}
	for s := int64(0); s < n; s++ {
		counts[RollExpedition(atk, s).Outcome]++
	}
	for _, o := range []ExpeditionOutcome{
		ExpeditionResources, ExpeditionShips, ExpeditionCombat, ExpeditionDelay,
		ExpeditionDarkMatter, ExpeditionFastReturn, ExpeditionNothing,
		ExpeditionBlackHole, ExpeditionBlackHoleLoot,
	} {
		if counts[o] == 0 {
			t.Errorf("outcome %q never occurred in %d rolls", o, n)
		}
	}
	assertShare(t, counts, n, ExpeditionCombat, 0.15, 0.02)
	assertShare(t, counts, n, ExpeditionBlackHole, 0.0168, 0.005)
	assertShare(t, counts, n, ExpeditionBlackHoleLoot, 0.017, 0.005)
	assertShare(t, counts, n, ExpeditionResources, 0.2028, 0.03)
	assertShare(t, counts, n, ExpeditionShips, 0.2879, 0.03)
	assertShare(t, counts, n, ExpeditionDarkMatter, 0.1465, 0.03)
	assertShare(t, counts, n, ExpeditionNothing, 0.079, 0.02)
	assertShare(t, counts, n, ExpeditionFastReturn, 0.04, 0.015)
}

// assertShare fails when an outcome's sampled share drifts beyond ±tol.
func assertShare(t *testing.T, counts map[ExpeditionOutcome]int, n int, o ExpeditionOutcome, want, tol float64) {
	t.Helper()
	got := float64(counts[o]) / float64(n)
	if got < want-tol || got > want+tol {
		t.Errorf("outcome %q share %.4f outside [%.4f,%.4f]", o, got, want-tol, want+tol)
	}
}

// TestExpeditionWeightsSumTo1 guards the mix against an accidental drift.
func TestExpeditionWeightsSumTo1(t *testing.T) {
	var total float64
	for _, w := range expeditionOutcomeWeights {
		total += w.weight
	}
	if total < 0.9999 || total > 1.0001 {
		t.Fatalf("expedition weights sum to %.4f, want 1.0", total)
	}
}

// TestExpeditionMessage ensures every outcome yields a non-empty message and
// combat reports name the faction.
func TestExpeditionMessage(t *testing.T) {
	for _, o := range []ExpeditionOutcome{
		ExpeditionResources, ExpeditionShips, ExpeditionCombat, ExpeditionDelay,
		ExpeditionDarkMatter, ExpeditionFastReturn, ExpeditionNothing,
		ExpeditionBlackHole, ExpeditionBlackHoleLoot,
	} {
		title, body := ExpeditionMessage(ExpeditionResult{Outcome: o})
		if title == "" || body == "" {
			t.Errorf("outcome %q produced an empty message", o)
		}
	}
	// Every flavour index must map to a non-empty body for every outcome.
	for _, o := range []ExpeditionOutcome{
		ExpeditionResources, ExpeditionShips, ExpeditionCombat, ExpeditionDelay,
		ExpeditionDarkMatter, ExpeditionFastReturn, ExpeditionNothing,
		ExpeditionBlackHole, ExpeditionBlackHoleLoot,
	} {
		for f := 0; f < 40; f++ {
			_, body := ExpeditionMessage(ExpeditionResult{Outcome: o, Flavor: f, NPC: NPCPirates})
			if body == "" {
				t.Fatalf("outcome %q flavour %d produced an empty body", o, f)
			}
		}
	}
	_, alien := ExpeditionMessage(ExpeditionResult{
		Outcome: ExpeditionCombat,
		NPC:     NPCAliens,
		Combat:  &CombatResult{Winner: "defender"},
	})
	if alien == "" {
		t.Fatal("alien combat message empty")
	}
}

// TestExpeditionLootCapped asserts a resource find never exceeds the hold.
func TestExpeditionLootCapped(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const cap = int64(5_000_000_000)
	for i := 0; i < 200; i++ {
		l := expeditionLoot(cap, rng)
		total := l.Metal + l.Crystal + l.Deuterium
		if total > cap {
			t.Fatalf("loot %d exceeds capacity %d", total, cap)
		}
		if l.Metal < 0 || l.Crystal < 0 || l.Deuterium < 0 {
			t.Fatalf("negative loot component: %+v", l)
		}
	}
	if got := expeditionLoot(0, rng); got != (Cost{}) {
		t.Fatalf("zero-capacity loot = %+v, want empty", got)
	}
}

// TestExpeditionRecovery asserts ship finds are non-empty and bounded.
func TestExpeditionRecovery(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	const pts = int64(1_000_000_000)
	for i := 0; i < 200; i++ {
		ships := expeditionRecovery(pts, rng)
		if len(ships) == 0 {
			t.Fatalf("empty recovery for %d points", pts)
		}
		for code, n := range ships {
			if n <= 0 {
				t.Fatalf("non-positive recovery %s=%d", code, n)
			}
		}
	}
	if expeditionRecovery(0, rng) != nil {
		t.Fatal("recovery for 0 points should be nil")
	}
}

// TestExpeditionEnemyMirror verifies the type-dependent mirror range, the
// template floor/ceil and the rolled research bonus by faction.
func TestExpeditionEnemyMirror(t *testing.T) {
	atk := expSampleAtk()
	fleet := map[string]int64{"227": 1000}
	gen := MirroredResearchBonus(atk.Techs)

	for seed := int64(0); seed < 400; seed++ {
		rng := rand.New(rand.NewSource(seed))
		enemy, flat := expeditionEnemy(fleet, atk, NPCPirates, rng)
		got := enemy["227"]
		if got < 600 || got > 690 {
			t.Fatalf("seed %d pirate mirror %d outside [600,690]", seed, got)
		}
		if flat < 0.1*gen-1e-9 || flat > 1.8*gen+1e-9 {
			t.Fatalf("seed %d pirate bonus %.2f outside [%.2f,%.2f]", seed, flat, 0.1*gen, 1.8*gen)
		}
		if _, ok := enemy["215"]; ok {
			t.Fatalf("seed %d pirate enemy carried alien-only 215", seed)
		}
	}

	for seed := int64(0); seed < 400; seed++ {
		rng := rand.New(rand.NewSource(seed))
		alien, flat := expeditionEnemy(fleet, atk, NPCAliens, rng)
		if m := alien["227"]; m < 850 || m > 940 {
			t.Fatalf("seed %d alien mirror %d outside [850,940]", seed, m)
		}
		if alien["203"] < 20 || alien["203"] > 1180 { // template floor + mirror
			t.Fatalf("seed %d alien template HC = %d out of range", seed, alien["203"])
		}
		if flat < 0.70*gen-1e-9 || flat > 2.5*gen+1e-9 {
			t.Fatalf("seed %d alien bonus %.2f outside [0.70,2.5]x%.2f", seed, flat, gen)
		}
	}
}

// TestExpeditionEnemyStrengthDistribution checks the strength rolls hit their
// rough percentiles: pirates skew low, aliens sit at/above our bonus.
func TestExpeditionEnemyStrengthDistribution(t *testing.T) {
	atk := expSampleAtk()
	fleet := map[string]int64{"227": 1000}
	gen := MirroredResearchBonus(atk.Techs)
	const n = 5000
	median := func(samples []float64) float64 {
		sort.Float64s(samples)
		return samples[len(samples)/2]
	}
	var pirate, alien []float64
	for s := int64(0); s < n; s++ {
		_, pf := expeditionEnemy(fleet, atk, NPCPirates, rand.New(rand.NewSource(s)))
		_, af := expeditionEnemy(fleet, atk, NPCAliens, rand.New(rand.NewSource(s)))
		pirate = append(pirate, pf/gen)
		alien = append(alien, af/gen)
	}
	if m := median(pirate); m < 0.55 || m > 0.85 {
		t.Errorf("pirate median roll %.2f outside [0.55,0.85]", m)
	}
	if m := median(alien); m < 1.0 || m > 1.35 {
		t.Errorf("alien median roll %.2f outside [1.0,1.35]", m)
	}
}

// TestExpeditionNPCShare checks the 84:16 pirate:alien combat split.
func TestExpeditionNPCShare(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const n = 40000
	pirates := 0
	for i := 0; i < n; i++ {
		if rollNPC(rng) == NPCPirates {
			pirates++
		}
	}
	share := float64(pirates) / n
	if share < 0.82 || share > 0.86 {
		t.Fatalf("pirate share %.3f outside [0.82,0.86]", share)
	}
}

// TestExpeditionDropRate checks the ~10 % drawing drop and that only catalog
// codes can drop.
func TestExpeditionDropRate(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	const n = 4000
	drops := 0
	for i := 0; i < n; i++ {
		code := rollExpeditionDrop(1_000_000_000, rng)
		if code == 0 {
			continue
		}
		if _, ok := UpgradeByCode(code); !ok {
			t.Fatalf("dropped unknown upgrade code %d", code)
		}
		drops++
	}
	rate := float64(drops) / n
	if rate < 0.05 || rate > 0.20 {
		t.Fatalf("drop rate %.3f outside [0.05,0.20]", rate)
	}
}

// TestExpeditionBlackHoleIsBare asserts a black hole carries no loot or ships.
func TestExpeditionBlackHoleIsBare(t *testing.T) {
	atk := expSampleAtk()
	for s := int64(0); s < 2000; s++ {
		res := RollExpedition(atk, s)
		if res.Outcome != ExpeditionBlackHole {
			continue
		}
		if res.Combat != nil || len(res.Ships) != 0 || res.Loot != (Cost{}) || res.DarkMatter != 0 {
			t.Fatalf("black hole carried payload: %+v", res)
		}
		return
	}
	t.Fatal("no black hole found in 2000 rolls")
}

// TestExpeditionDarkMatterScales: DM finds scale with the fleet's points, so a
// 10× fleet yields a ~10× median (and stays above the floor).
func TestExpeditionDarkMatterScales(t *testing.T) {
	const n = 4000
	sample := func(points int64) []int64 {
		rng := rand.New(rand.NewSource(99))
		out := make([]int64, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, expeditionDarkMatter(points, rng))
		}
		return out
	}
	small := sample(4.0e10)
	large := sample(4.0e11)
	median := func(v []int64) int64 {
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		return v[len(v)/2]
	}
	ms, ml := median(small), median(large)
	if ms < darkMatterFloor {
		t.Fatalf("small fleet median DM %d below floor", ms)
	}
	ratio := float64(ml) / float64(ms)
	if ratio < 8 || ratio > 12 {
		t.Fatalf("DM scaling ratio %.2f, want ~10", ratio)
	}
	// A no-points fleet keeps the legacy flat range.
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 200; i++ {
		if dm := expeditionDarkMatter(0, rng); dm < 100 || dm > 5000 {
			t.Fatalf("flat DM %d outside [100,5000]", dm)
		}
	}
}

// TestExpeditionBlackHoleLoot: the positive black hole yields loot capped by the
// hold and never wipes anything (engine branch is the default path).
func TestExpeditionBlackHoleLoot(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	const capacity = int64(1_000_000_000)
	sawBoost := false
	for i := 0; i < 400; i++ {
		l := expeditionBlackHoleLoot(capacity, rng)
		total := l.Metal + l.Crystal + l.Deuterium
		if total > capacity {
			t.Fatalf("BH-loot %d exceeds capacity %d", total, capacity)
		}
		if total > 0 {
			sawBoost = true
		}
	}
	if !sawBoost {
		t.Fatal("black hole loot never produced resources")
	}
	if got := expeditionBlackHoleLoot(0, rng); got != (Cost{}) {
		t.Fatalf("zero-capacity BH-loot = %+v, want empty", got)
	}
}

// TestMirroredResearchBonus picks the strongest general W/S/A bonus.
func TestMirroredResearchBonus(t *testing.T) {
	if got, want := MirroredResearchBonus(CombatTechs{Weapons: 10, Shield: 12, Armour: 11}), float64(TechBonus(12)); got != want {
		t.Fatalf("MirroredResearchBonus = %v, want %v", got, want)
	}
}

// TestFleetPoints locks the metal+crystal point sum used for scaling and tiers.
func TestFleetPoints(t *testing.T) {
	// 227 = 30M+10M = 40M, 219 = 1M+0.6M = 1.6M.
	got := FleetPoints(map[string]int64{"227": 2, "219": 5})
	want := int64(2*40_000_000 + 5*1_600_000)
	if got != want {
		t.Fatalf("FleetPoints = %d, want %d", got, want)
	}
	if FleetPoints(nil) != 0 {
		t.Fatal("FleetPoints(nil) should be 0")
	}
}
