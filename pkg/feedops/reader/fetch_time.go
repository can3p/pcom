package reader

import (
	"math"
	"time"
)

const (
	MinFetchInterval = 60
	MaxFetchInterval = 60 * 24
)

func CalculateNextFetchTime(consecutiveEmptyFetches int, avgItemsPerDay float64, wasManual bool) time.Time {
	if wasManual {
		return time.Now().Add(time.Hour)
	}

	var baseInterval int

	// Calculate base interval based on activity
	switch {
	case avgItemsPerDay > 10:
		baseInterval = 60 // Very active feeds
	case avgItemsPerDay > 2:
		baseInterval = 180 // Moderately active feeds
	case avgItemsPerDay > 0.5:
		baseInterval = 360 // Less active feeds
	default:
		baseInterval = 720 // Rarely updated feeds
	}

	// Adjust for consecutive empty fetches
	emptyFetchMultiplier := 1 + int(math.Min(float64(consecutiveEmptyFetches), 3))
	baseInterval *= emptyFetchMultiplier

	// Ensure we stay within bounds
	if baseInterval < MinFetchInterval {
		baseInterval = MinFetchInterval
	}
	if baseInterval > MaxFetchInterval {
		baseInterval = MaxFetchInterval
	}

	return time.Now().Add(time.Duration(baseInterval) * time.Minute)
}
