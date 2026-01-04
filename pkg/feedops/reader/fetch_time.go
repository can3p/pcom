package reader

import (
	"math"
	"time"
)

const (
	ManualFetchInterval = time.Hour
	MinFetchInterval    = time.Hour
	MaxFetchInterval    = 24 * time.Hour
	ErrorFetchInterval  = time.Hour
)

func CalculateNextFetchTime(consecutiveEmptyFetches int, avgItemsPerDay float64, wasManual bool) time.Time {
	if wasManual {
		return time.Now().Add(ManualFetchInterval)
	}

	var baseIntervalMins int

	// Calculate base interval based on activity
	switch {
	case avgItemsPerDay > 10:
		baseIntervalMins = 60 // Very active feeds
	case avgItemsPerDay > 2:
		baseIntervalMins = 180 // Moderately active feeds
	case avgItemsPerDay > 0.5:
		baseIntervalMins = 360 // Less active feeds
	default:
		baseIntervalMins = 720 // Rarely updated feeds
	}

	// Adjust for consecutive empty fetches
	emptyFetchMultiplier := 1 + int(math.Min(float64(consecutiveEmptyFetches), 3))
	baseIntervalMins *= emptyFetchMultiplier

	// Ensure we stay within bounds
	if baseIntervalMins < int(MinFetchInterval.Minutes()) {
		baseIntervalMins = int(MinFetchInterval.Minutes())
	}
	if baseIntervalMins > int(MaxFetchInterval.Minutes()) {
		baseIntervalMins = int(MaxFetchInterval.Minutes())
	}

	return time.Now().Add(time.Duration(baseIntervalMins) * time.Minute)
}
