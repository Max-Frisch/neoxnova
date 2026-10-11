package web

import (
	"testing"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

func TestTechNode(t *testing.T) {
	all := map[string]int{}
	t.Run("building", func(t *testing.T) {
		n, ok := techNode("shipyard", game.TechTree["shipyard"], map[string]int{"shipyard": 3}, nil, map[string]int{"shipyard": 3})
		if !ok || n.Category != "Buildings" || n.Name != "Shipyard" || n.ID != 21 || n.Level != 3 || !n.Leveled {
			t.Fatalf("unexpected building node: %+v ok=%v", n, ok)
		}
		if n.Unlocked {
			t.Fatalf("shipyard should be locked at level 3 (needs robot factory 2): %+v", n.Requires)
		}
	})
	t.Run("research unlocked", func(t *testing.T) {
		levels := map[string]int{"research_lab": 3}
		n, ok := techNode("espionage_tech", game.TechTree["espionage_tech"], nil, map[string]int{"espionage_tech": 1}, levels)
		if !ok || n.Category != "Research" || n.Name != "Spy Technology" || n.ID != 106 {
			t.Fatalf("unexpected research node: %+v ok=%v", n, ok)
		}
		if !n.Unlocked {
			t.Fatalf("spy tech should be unlocked with research_lab 3: %+v", n.Requires)
		}
	})
	t.Run("ships and defenses are unlevelled", func(t *testing.T) {
		n, ok := techNode("202", game.TechTree["202"], nil, nil, all)
		if !ok || n.Category != "Ships" || n.Leveled || n.Name != "Light Cargo" {
			t.Fatalf("unexpected ship node: %+v ok=%v", n, ok)
		}
		n, ok = techNode("401", game.TechTree["401"], nil, nil, all)
		if !ok || n.Category != "Defenses" || n.Leveled {
			t.Fatalf("unexpected defense node: %+v ok=%v", n, ok)
		}
	})
	t.Run("moon structure", func(t *testing.T) {
		n, ok := techNode("moon_base", game.TechTree["moon_base"], nil, nil, all)
		if !ok || n.Category != "Moon" || n.Name != "Moon base" {
			t.Fatalf("unexpected moon node: %+v ok=%v", n, ok)
		}
	})
	t.Run("unknown code skipped", func(t *testing.T) {
		if _, ok := techNode("does_not_exist", nil, nil, nil, all); ok {
			t.Fatal("unknown code should not produce a node")
		}
	})
}

func TestSimSideViewFiltersAndSorts(t *testing.T) {
	v := simSideView(game.SideReport{
		Initial:   map[string]int64{"204": 3, "202": 0, "207": 1},
		Lost:      map[string]int64{"204": 1},
		Remaining: map[string]int64{"204": 2, "207": 1},
	})
	if len(v.Initial) != 2 {
		t.Fatalf("zero-count units should be filtered: %+v", v.Initial)
	}
	// Sorted by display name: "Battleship" before "Light Fighter".
	if v.Initial[0].Name != "Battleship" || v.Initial[1].Name != "Light Fighter" {
		t.Fatalf("rows not name-sorted: %+v", v.Initial)
	}
	if len(v.Lost) != 1 || v.Lost[0].Name != "Light Fighter" {
		t.Fatalf("unexpected losses: %+v", v.Lost)
	}
}

func TestSimByUnit(t *testing.T) {
	rows := []SimUnitCount{{Name: "Light Fighter", Count: 7}}
	if got := simByUnit(rows, "Light Fighter"); got != 7 {
		t.Fatalf("simByUnit = %d, want 7", got)
	}
	if got := simByUnit(rows, "Nope"); got != 0 {
		t.Fatalf("simByUnit missing = %d, want 0", got)
	}
}

func TestFormatters(t *testing.T) {
	if got := bonusStr(1.5); got != "+1.5%" {
		t.Fatalf("bonusStr = %q", got)
	}
	if got := pct(1.0); got != "100%" {
		t.Fatalf("pct(1) = %q", got)
	}
	if got := pct(0.75); got != "75%" {
		t.Fatalf("pct(0.75) = %q", got)
	}
	if got := countStr(0); got != "0" {
		t.Fatalf("countStr(0) = %q", got)
	}
	if got := countStr(-5); got != "0" {
		t.Fatalf("countStr(-5) = %q", got)
	}
	if got := countStr(123); got != "123" {
		t.Fatalf("countStr(123) = %q", got)
	}
}

func TestLotExpirySecsFlooredAtZero(t *testing.T) {
	if got := lotExpirySecs(models.MarketLot{ExpiresAt: time.Now().Add(-time.Hour)}); got != 0 {
		t.Fatalf("expired lot = %d, want 0", got)
	}
	if got := lotExpirySecs(models.MarketLot{ExpiresAt: time.Now().Add(time.Hour)}); got <= 0 || got > 3600 {
		t.Fatalf("future lot = %d, want (0,3600]", got)
	}
}
