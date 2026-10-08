package game

import (
	"math/rand"
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
// rates hold: combat 15 %, black hole 0.5 %.
func TestExpeditionOutcomeMix(t *testing.T) {
	atk := expSampleAtk()
	const n = 20000
	counts := map[ExpeditionOutcome]int{}
	for s := int64(0); s < n; s++ {
		counts[RollExpedition(atk, s).Outcome]++
	}
	for _, o := range []ExpeditionOutcome{
		ExpeditionResources, ExpeditionShips, ExpeditionCombat, ExpeditionDelay,
		ExpeditionDarkMatter, ExpeditionFastReturn, ExpeditionNothing, ExpeditionBlackHole,
	} {
		if counts[o] == 0 {
			t.Errorf("outcome %q never occurred in %d rolls", o, n)
		}
	}
	bh := float64(counts[ExpeditionBlackHole]) / n
	if bh < 0.002 || bh > 0.012 {
		t.Errorf("black-hole rate %.4f outside [0.002,0.012]", bh)
	}
	fight := float64(counts[ExpeditionCombat]) / n
	if fight < 0.12 || fight > 0.18 {
		t.Errorf("combat rate %.4f outside [0.12,0.18]", fight)
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
		ExpeditionDarkMatter, ExpeditionFastReturn, ExpeditionNothing, ExpeditionBlackHole,
	} {
		title, body := ExpeditionMessage(ExpeditionResult{Outcome: o})
		if title == "" || body == "" {
			t.Errorf("outcome %q produced an empty message", o)
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

// TestExpeditionEnemyMirror verifies the mirrored range, the template floor and
// the rolled research bonus by faction.
func TestExpeditionEnemyMirror(t *testing.T) {
	atk := expSampleAtk()
	fleet := map[string]int64{"227": 1000}
	gen := MirroredResearchBonus(atk.Techs)

	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		enemy, flat := expeditionEnemy(fleet, atk, NPCPirates, rng)
		got := enemy["227"]
		if got < 500 || got > 900 {
			t.Fatalf("seed %d pirate mirror %d outside [500,900]", seed, got)
		}
		if flat < 0.1*gen-1e-9 || flat > 1.8*gen+1e-9 {
			t.Fatalf("seed %d pirate bonus %.2f outside [%.2f,%.2f]", seed, flat, 0.1*gen, 1.8*gen)
		}
	}

	rng := rand.New(rand.NewSource(7))
	alien, flat := expeditionEnemy(fleet, atk, NPCAliens, rng)
	if alien["203"] < 40 { // template floor
		t.Fatalf("alien template HC = %d, want >= 40", alien["203"])
	}
	if flat < 0.5*gen-1e-9 || flat > 2.4*gen+1e-9 {
		t.Fatalf("alien bonus %.2f outside [0.5,2.4]x%.2f", flat, gen)
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
