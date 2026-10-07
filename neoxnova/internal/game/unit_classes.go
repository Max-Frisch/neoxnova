// Code generated from testdata/niburus_unit_classes.json; DO NOT EDIT.

package game

// UnitClass is the set of Arsenal upgrade classes on a unit's information card
// (game.php?page=information&id=<code>). Values are normalised to the catalog
// class keys used by internal/game/arsenal.go: weapon
// {laser,ion,plasma,gravitational,standard}, armor/shield {light,medium,heavy},
// engine {combustion,impulse,hyperspace}. An empty string means the card
// declares no class for that slot (cargo ships have no weapon, defenses no
// engine).
//
// A unit can carry SEVERAL weapon types, each contributing its own base attack
// (e.g. 225 Galleon = Laser 10,000 + Ion 10,000); those are listed in Weapons.
// Weapon is the first (primary) weapon class, kept for callers that only need
// a single class.
type UnitClass struct {
	Weapon  string
	Weapons []WeaponClass
	Armor   string
	Shield  string
	Engine  string
}

// WeaponClass is one weapon component of a unit: its Arsenal class and the
// base attack power it contributes (before tech/upgrade bonuses).
type WeaponClass struct {
	Class  string
	Attack int
}

// unitClasses holds the declared classes for every ship (202-228) and defense
// (401-419). Codes absent from the map have no information card.
var unitClasses = map[string]UnitClass{
	"202": {Weapon: "", Armor: "light", Shield: "light", Engine: "combustion"},
	"203": {Weapon: "", Armor: "light", Shield: "light", Engine: "combustion"},
	"204": {Weapon: "standard", Weapons: []WeaponClass{{Class: "standard", Attack: 50}}, Armor: "light", Shield: "light", Engine: "combustion"},
	"205": {Weapon: "standard", Weapons: []WeaponClass{{Class: "standard", Attack: 150}}, Armor: "light", Shield: "light", Engine: "impulse"},
	"206": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 400}}, Armor: "medium", Shield: "medium", Engine: "impulse"},
	"207": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 700}}, Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"208": {Weapon: "", Armor: "light", Shield: "light", Engine: "impulse"},
	"209": {Weapon: "", Armor: "light", Shield: "light", Engine: "combustion"},
	"210": {Weapon: "", Armor: "light", Shield: "", Engine: "combustion"},
	"211": {Weapon: "plasma", Weapons: []WeaponClass{{Class: "plasma", Attack: 1400}}, Armor: "medium", Shield: "medium", Engine: "impulse"},
	"212": {Weapon: "", Armor: "light", Shield: "", Engine: ""},
	"213": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 1500}, {Class: "plasma", Attack: 500}}, Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"214": {Weapon: "gravitational", Weapons: []WeaponClass{{Class: "gravitational", Attack: 150000}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"215": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 1400}}, Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"216": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 20250}, {Class: "gravitational", Attack: 114750}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"217": {Weapon: "standard", Weapons: []WeaponClass{{Class: "standard", Attack: 40}}, Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"218": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 780000}, {Class: "gravitational", Attack: 4420000}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"219": {Weapon: "", Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"220": {Weapon: "", Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"221": {Weapon: "plasma", Weapons: []WeaponClass{{Class: "plasma", Attack: 1800000}, {Class: "gravitational", Attack: 4200000}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"222": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 600000}, {Class: "plasma", Attack: 1000000}, {Class: "gravitational", Attack: 2400000}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"223": {Weapon: "", Armor: "light", Shield: "light", Engine: "combustion"},
	"224": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 1225}, {Class: "ion", Attack: 2975}}, Armor: "medium", Shield: "medium", Engine: "hyperspace"},
	"225": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 10000}, {Class: "ion", Attack: 10000}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"226": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 19500}, {Class: "ion", Attack: 45500}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"227": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 117500}, {Class: "ion", Attack: 188000}, {Class: "gravitational", Attack: 164500}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"228": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 225000}, {Class: "ion", Attack: 450000}, {Class: "plasma", Attack: 300000}, {Class: "gravitational", Attack: 525000}}, Armor: "heavy", Shield: "heavy", Engine: "hyperspace"},
	"401": {Weapon: "standard", Weapons: []WeaponClass{{Class: "standard", Attack: 80}}, Armor: "light", Shield: "light", Engine: ""},
	"402": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 100}}, Armor: "light", Shield: "light", Engine: ""},
	"403": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 250}}, Armor: "medium", Shield: "light", Engine: ""},
	"404": {Weapon: "standard", Weapons: []WeaponClass{{Class: "standard", Attack: 1100}}, Armor: "medium", Shield: "medium", Engine: ""},
	"405": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 150}}, Armor: "medium", Shield: "medium", Engine: ""},
	"406": {Weapon: "plasma", Weapons: []WeaponClass{{Class: "plasma", Attack: 3000}}, Armor: "medium", Shield: "medium", Engine: ""},
	"407": {Weapon: "", Armor: "light", Shield: "heavy", Engine: ""},
	"408": {Weapon: "", Armor: "medium", Shield: "heavy", Engine: ""},
	"409": {Weapon: "", Armor: "heavy", Shield: "heavy", Engine: ""},
	"410": {Weapon: "gravitational", Weapons: []WeaponClass{{Class: "gravitational", Attack: 500000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"411": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 150000000}, {Class: "ion", Attack: 150000000}, {Class: "plasma", Attack: 250000000}, {Class: "gravitational", Attack: 450000000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"412": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 200000}, {Class: "gravitational", Attack: 200000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"413": {Weapon: "plasma", Weapons: []WeaponClass{{Class: "plasma", Attack: 225000}, {Class: "gravitational", Attack: 675000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"414": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 1875000}, {Class: "gravitational", Attack: 625000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"415": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 800000}, {Class: "plasma", Attack: 2400000}, {Class: "gravitational", Attack: 4800000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"416": {Weapon: "laser", Weapons: []WeaponClass{{Class: "laser", Attack: 2700}, {Class: "ion", Attack: 6300}}, Armor: "medium", Shield: "medium", Engine: ""},
	"417": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 6000}, {Class: "plasma", Attack: 6000}}, Armor: "medium", Shield: "medium", Engine: ""},
	"418": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 10500}, {Class: "plasma", Attack: 17500}, {Class: "gravitational", Attack: 42000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
	"419": {Weapon: "ion", Weapons: []WeaponClass{{Class: "ion", Attack: 302000}, {Class: "plasma", Attack: 302000}, {Class: "gravitational", Attack: 906000}}, Armor: "heavy", Shield: "heavy", Engine: ""},
}

// UnitClassOf returns the declared upgrade classes for a unit code.
func UnitClassOf(code string) (UnitClass, bool) {
	c, ok := unitClasses[code]
	return c, ok
}

// UpgradeCodeForClass resolves a unit's declared class within an upgrade group
// (weapon|armor|shield|engine) to the Arsenal upgrade code that applies to it,
// or 0 when no catalog upgrade covers that class (e.g. "standard" weapons).
func UpgradeCodeForClass(group, class string) int {
	if class == "" {
		return 0
	}
	return upgradeCodeByClass[group+"/"+class]
}
