package game

import "math"

// Espionage ("spy") model, following the classic OGame formula. A spy report is
// produced only by Espionage Probes (unit 210) and reveals more sections as the
// information score grows. The score is dominated by the espionage-research
// levels: a higher attacker level gives a quadratic discount on the probes
// required, while a higher defender level raises them quadratically.
//
//	score = probes + (yourTech - enemyTech) * |yourTech - enemyTech|
//
// Section thresholds (resources are always revealed):
//
//	score >= 2  fleet
//	score >= 3  defense
//	score >= 5  buildings
//	score >= 7  research
//
// The thresholds reproduce the reference required-probe table (e.g. a defender
// +3 levels above needs 11 probes to reveal fleet, 16 to reveal research).
const (
	EspionageFleetThreshold     = 2
	EspionageDefenseThreshold   = 3
	EspionageBuildingsThreshold = 5
	EspionageResearchThreshold  = 7
)

// EspionageScore returns the information score of a spy mission.
func EspionageScore(probes, yourTech, enemyTech int) int {
	d := yourTech - enemyTech
	return probes + d*absInt(d)
}

// EspionageReveals reports which extra sections an information score unlocks.
// Resources are always revealed and are not represented here.
func EspionageReveals(score int) (fleet, defense, buildings, research bool) {
	return score >= EspionageFleetThreshold,
		score >= EspionageDefenseThreshold,
		score >= EspionageBuildingsThreshold,
		score >= EspionageResearchThreshold
}

// CounterEspionageCoefficient is the per-(probe x defender-ship) chance of a
// probe being detected, before the tech-level factor. The exact reference
// formula is undocumented and strongly randomised; this follows the widely
// published o-tools approximation (0.25% per probe per defender ship) with the
// tech advantage on the defender's side.
const CounterEspionageCoefficient = 0.0025

// CounterEspionageChance estimates the probability that the defending planet
// shoots down a spy probe, given both espionage levels, the number of probes
// sent and the defender's ship count (defenses do not count). Higher defender
// espionage and more probes/ships raise it; higher attacker espionage lowers it.
// Clamped to [0, 1]. A planet with no ships has a true 0% chance.
func CounterEspionageChance(attackerTech, defenderTech, probes, defenderShipCount int) float64 {
	if probes <= 0 || defenderShipCount <= 0 {
		return 0
	}
	techFactor := math.Pow(2, float64(defenderTech-attackerTech))
	chance := CounterEspionageCoefficient * techFactor * float64(probes) * float64(defenderShipCount)
	if chance > 1 {
		return 1
	}
	if chance < 0 {
		return 0
	}
	return chance
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
