package game

import "math"

// StorageCapacity returns the storage capacity for a resource at the given
// storage-building level, following the classic OGame curve
//
//	capacity(L) = 5000 * floor(2.5 * e^(20*L/33))
//
// scaled by the universe's resource production multiplier so a 10000x server
// holds 10000x as much. The level-0 base is therefore 100,000,000 on the
// reference server (resource_speed 10000) and 10,000 on a 1x server. Level 10 there holds
// exactly 53,550,000,000, matching the captured live resources page.
func StorageCapacity(level int, resourceSpeed float64) int64 {
	if level < 0 {
		level = 0
	}
	if resourceSpeed <= 0 {
		resourceSpeed = 1.0
	}
	base := math.Floor(2.5 * math.Exp(20.0*float64(level)/33.0))
	return int64(5000 * base * resourceSpeed)
}
