package web

import (
	"net/http"
	"sort"
	"strconv"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// empireColumn is one catalog entry shown as a matrix row label.
type empireColumn struct {
	Code string
	Name string
}

// empireRow is one rendered matrix row: its label, the Σ total and one formatted
// value per celestial (in the same order as EmpireData.Celestials).
type empireRow struct {
	Label  string
	Total  string
	Values []string
}

// empireSection groups rows under a header and carries the data-section key the
// client-side All/Building/Fleet/Defense filter toggles.
type empireSection struct {
	Key   string
	Title string
	Rows  []empireRow
}

// EmpireData backs the Empire overview page (live page=imperium).
type EmpireData struct {
	Celestials []models.EmpireCelestial
	Sections   []empireSection
}

// EmpirePage renders the account-wide Empire overview (page=imperium) anchored
// to the homeworld so the shared chrome renders normally.
func (h *Handler) EmpirePage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Empire")
	celestials, err := h.planets.EmpireOverview(r.Context(), userID)
	if err != nil {
		sh.Error = "Could not load the Empire overview"
	}
	h.render(w, r, http.StatusOK, Page(sh, EmpireBody(sh, buildEmpireData(celestials))))
}

// buildEmpireData folds the store rows into the sectioned matrix the template
// renders: Resources and Production per hour, then the Buildings / Ships /
// Defenses matrices with their Σ totals.
func buildEmpireData(celestials []models.EmpireCelestial) EmpireData {
	d := EmpireData{Celestials: celestials}
	d.Sections = []empireSection{
		{Key: "resources", Title: "Resources", Rows: []empireRow{
			empireFloatRow("Metal", celestials, func(c models.EmpireCelestial) float64 { return c.Metal }),
			empireFloatRow("Crystal", celestials, func(c models.EmpireCelestial) float64 { return c.Crystal }),
			empireFloatRow("Deuterium", celestials, func(c models.EmpireCelestial) float64 { return c.Deuterium }),
			empireIntRow("Energy", celestials, func(c models.EmpireCelestial) int64 { return int64(c.EnergyAvailable) }),
		}},
		{Key: "production", Title: "Production per hour", Rows: []empireRow{
			empireFloatRow("Metal", celestials, func(c models.EmpireCelestial) float64 { return c.MetalPerHour }),
			empireFloatRow("Crystal", celestials, func(c models.EmpireCelestial) float64 { return c.CrystalPerHour }),
			empireFloatRow("Deuterium", celestials, func(c models.EmpireCelestial) float64 { return c.DeuteriumPerHour }),
			empireFloatRow("Dark Matter", celestials, func(c models.EmpireCelestial) float64 { return c.DarkMatterPerHour }),
		}},
		{Key: "building", Title: "Buildings", Rows: empireBuildingRows(celestials)},
		{Key: "fleet", Title: "Ships", Rows: empireUnitRows(celestials, empireUnitColumns(game.Ships), func(c models.EmpireCelestial) map[string]int64 { return c.Ships })},
		{Key: "defense", Title: "Defenses", Rows: empireUnitRows(celestials, empireUnitColumns(game.Defenses), func(c models.EmpireCelestial) map[string]int64 { return c.Defenses })},
	}
	return d
}

// empireFloatRow formats a float64 metric (resources, hourly production) with
// thousands separators and sums the Σ column.
func empireFloatRow(label string, celestials []models.EmpireCelestial, get func(models.EmpireCelestial) float64) empireRow {
	row := empireRow{Label: label, Values: make([]string, len(celestials))}
	var total float64
	for i, c := range celestials {
		v := get(c)
		row.Values[i] = fnumF(v)
		total += v
	}
	row.Total = fnumF(total)
	return row
}

// empireIntRow formats an int64 metric (energy, building levels, unit counts)
// and sums the Σ column.
func empireIntRow(label string, celestials []models.EmpireCelestial, get func(models.EmpireCelestial) int64) empireRow {
	row := empireRow{Label: label, Values: make([]string, len(celestials))}
	var total int64
	for i, c := range celestials {
		v := get(c)
		row.Values[i] = fnum(v)
		total += v
	}
	row.Total = fnum(total)
	return row
}

// empireBuildingColumns merges the planet and moon-only building catalogs into
// one row order (by numeric id), matching the live Buildings matrix.
func empireBuildingColumns() []empireColumn {
	defs := make([]game.StructureDef, 0, len(game.Structures)+len(game.MoonOnlyStructures))
	for _, d := range game.Structures {
		defs = append(defs, d)
	}
	for _, d := range game.MoonOnlyStructures {
		defs = append(defs, d)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	out := make([]empireColumn, len(defs))
	for i, d := range defs {
		out[i] = empireColumn{Code: d.Code, Name: d.Name}
	}
	return out
}

// empireUnitColumns orders a ship or defense catalog by numeric code.
func empireUnitColumns(units map[string]game.UnitDef) []empireColumn {
	codes := make([]string, 0, len(units))
	for code := range units {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]empireColumn, len(codes))
	for i, code := range codes {
		out[i] = empireColumn{Code: code, Name: units[code].Name}
	}
	return out
}

// empireBuildingRows builds one row per building column with each celestial's level.
func empireBuildingRows(celestials []models.EmpireCelestial) []empireRow {
	cols := empireBuildingColumns()
	rows := make([]empireRow, len(cols))
	for i, col := range cols {
		code := col.Code
		rows[i] = empireIntRow(col.Name, celestials, func(c models.EmpireCelestial) int64 {
			return int64(c.Buildings[code])
		})
	}
	return rows
}

// empireUnitRows builds one row per ship/defense column with each celestial's count.
func empireUnitRows(celestials []models.EmpireCelestial, cols []empireColumn, pick func(models.EmpireCelestial) map[string]int64) []empireRow {
	rows := make([]empireRow, len(cols))
	for i, col := range cols {
		code := col.Code
		rows[i] = empireIntRow(col.Name, celestials, func(c models.EmpireCelestial) int64 {
			return pick(c)[code]
		})
	}
	return rows
}

// empireCelestialHref links a celestial column to its overview page.
func empireCelestialHref(id int64) string {
	return "/overview/" + strconv.FormatInt(id, 10)
}
