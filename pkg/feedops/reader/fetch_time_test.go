package reader_test

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/stretchr/testify/require"
)

func TestCalculateNextFetchTime_ManualFetch(t *testing.T) {
	t.Parallel()

	before := time.Now()
	nextTime := reader.CalculateNextFetchTime(0, 5.0, true)
	after := time.Now()

	expectedMin := before.Add(reader.ManualFetchInterval)
	expectedMax := after.Add(reader.ManualFetchInterval)

	require.True(t, nextTime.After(expectedMin) || nextTime.Equal(expectedMin),
		"next fetch time should be after or equal to min expected")
	require.True(t, nextTime.Before(expectedMax) || nextTime.Equal(expectedMax),
		"next fetch time should be before or equal to max expected")
}

func TestCalculateNextFetchTime_VeryActiveFeed(t *testing.T) {
	t.Parallel()

	// Feed with > 10 items per day should fetch every 60 minutes
	nextTime := reader.CalculateNextFetchTime(0, 15.0, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should be approximately 60 minutes, allow 5 second variance
	require.Greater(t, diff, 59*time.Minute+55*time.Second)
	require.Less(t, diff, 60*time.Minute+5*time.Second)
}

func TestCalculateNextFetchTime_ModeratelyActiveFeed(t *testing.T) {
	t.Parallel()

	// Feed with 2-10 items per day should fetch every 180 minutes (3 hours)
	nextTime := reader.CalculateNextFetchTime(0, 5.0, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should be approximately 180 minutes, allow 5 second variance
	require.Greater(t, diff, 179*time.Minute+55*time.Second)
	require.Less(t, diff, 180*time.Minute+5*time.Second)
}

func TestCalculateNextFetchTime_LessActiveFeed(t *testing.T) {
	t.Parallel()

	// Feed with 0.5-2 items per day should fetch every 360 minutes (6 hours)
	nextTime := reader.CalculateNextFetchTime(0, 1.0, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should be approximately 360 minutes, allow 5 second variance
	require.Greater(t, diff, 359*time.Minute+55*time.Second)
	require.Less(t, diff, 360*time.Minute+5*time.Second)
}

func TestCalculateNextFetchTime_RarelyUpdatedFeed(t *testing.T) {
	t.Parallel()

	// Feed with < 0.5 items per day should fetch every 720 minutes (12 hours)
	nextTime := reader.CalculateNextFetchTime(0, 0.1, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should be approximately 720 minutes, allow 5 second variance
	require.Greater(t, diff, 719*time.Minute+55*time.Second)
	require.Less(t, diff, 720*time.Minute+5*time.Second)
}

func TestCalculateNextFetchTime_ConsecutiveEmptyFetches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		consecutiveEmpties int
		avgItemsPerDay     float64
		expectedMultiplier int
	}{
		{"0 empty fetches", 0, 5.0, 1},  // base: 180, multiplier: 1
		{"1 empty fetch", 1, 5.0, 2},    // base: 180, multiplier: 2
		{"2 empty fetches", 2, 5.0, 3},  // base: 180, multiplier: 3
		{"3 empty fetches", 3, 5.0, 4},  // base: 180, multiplier: 4
		{"4+ empty fetches", 4, 5.0, 4}, // base: 180, multiplier: 4 (capped at 3)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nextTime := reader.CalculateNextFetchTime(tt.consecutiveEmpties, tt.avgItemsPerDay, false)
			now := time.Now()
			diff := nextTime.Sub(now)

			// For moderately active feeds: base is 180 minutes
			expectedMinutes := 180 * tt.expectedMultiplier
			expectedMin := time.Duration(expectedMinutes)*time.Minute - 5*time.Second
			expectedMax := time.Duration(expectedMinutes)*time.Minute + 5*time.Second

			require.Greater(t, diff, expectedMin,
				"should account for consecutive empty fetches")
			require.Less(t, diff, expectedMax,
				"should account for consecutive empty fetches")
		})
	}
}

func TestCalculateNextFetchTime_MinBound(t *testing.T) {
	t.Parallel()

	// Extremely active feed should still respect MinFetchInterval
	nextTime := reader.CalculateNextFetchTime(0, 1000.0, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should not go below MinFetchInterval (1 hour)
	require.Greater(t, diff, reader.MinFetchInterval-5*time.Second)
}

func TestCalculateNextFetchTime_MaxBound(t *testing.T) {
	t.Parallel()

	// Very inactive feed with many empty fetches should not exceed MaxFetchInterval
	nextTime := reader.CalculateNextFetchTime(10, 0.01, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should not go above MaxFetchInterval (24 hours)
	require.Less(t, diff, reader.MaxFetchInterval+5*time.Second)
}

func TestCalculateNextFetchTime_EdgeCase_ZeroItems(t *testing.T) {
	t.Parallel()

	// Feed with 0 items per day
	nextTime := reader.CalculateNextFetchTime(0, 0.0, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Should be at minimum interval (720 minutes, same as "rarely updated")
	require.Greater(t, diff, 719*time.Minute+55*time.Second)
	require.Less(t, diff, 720*time.Minute+5*time.Second)
}

func TestCalculateNextFetchTime_EdgeCase_BoundaryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		avgItemsPerDay float64
		expectedMins   int
	}{
		{"Not quite 10", 9.9, 180},    // just under 10: moderately active
		{"Just over 10", 10.1, 60},    // crosses to "very active"
		{"Just over 2", 2.1, 180},     // crosses to "moderately active"
		{"Not quite 2", 1.9, 360},     // just under 2: less active
		{"Just over 0.5", 0.6, 360},   // crosses to "less active"
		{"Just under 0.5", 0.49, 720}, // goes to "rarely updated"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nextTime := reader.CalculateNextFetchTime(0, tt.avgItemsPerDay, false)
			now := time.Now()
			diff := nextTime.Sub(now)

			expectedMin := time.Duration(tt.expectedMins)*time.Minute - 5*time.Second
			expectedMax := time.Duration(tt.expectedMins)*time.Minute + 5*time.Second

			require.Greater(t, diff, expectedMin)
			require.Less(t, diff, expectedMax)
		})
	}
}

func TestCalculateNextFetchTime_CombinedEmptyAndActivity(t *testing.T) {
	t.Parallel()

	// Very active feed with 2 empty fetches
	nextTime := reader.CalculateNextFetchTime(2, 15.0, false)
	now := time.Now()
	diff := nextTime.Sub(now)

	// Base: 60 minutes (very active), multiplier: 3 (2 empty fetches) = 180 minutes
	expectedMin := 179*time.Minute + 55*time.Second
	expectedMax := 180*time.Minute + 5*time.Second

	require.Greater(t, diff, expectedMin)
	require.Less(t, diff, expectedMax)
}

func TestCalculateNextFetchTime_ManualOverridesAll(t *testing.T) {
	t.Parallel()

	// Manual fetch should always return ManualFetchInterval regardless of other params
	nextTime1 := reader.CalculateNextFetchTime(100, 0.01, true)
	nextTime2 := reader.CalculateNextFetchTime(0, 100.0, true)
	nextTime3 := reader.CalculateNextFetchTime(5, 50.0, true)

	now := time.Now()

	for _, nextTime := range []time.Time{nextTime1, nextTime2, nextTime3} {
		diff := nextTime.Sub(now)
		require.Greater(t, diff, reader.ManualFetchInterval-5*time.Second)
		require.Less(t, diff, reader.ManualFetchInterval+5*time.Second)
	}
}

func TestCalculateNextFetchTime_HighEmptyFetchCount(t *testing.T) {
	t.Parallel()

	// Empty fetch count > 3 should be capped at multiplier 4
	nextTime1 := reader.CalculateNextFetchTime(3, 5.0, false)
	nextTime2 := reader.CalculateNextFetchTime(5, 5.0, false)
	nextTime3 := reader.CalculateNextFetchTime(10, 5.0, false)

	now := time.Now()

	// All should have the same result (capped multiplier)
	diff1 := nextTime1.Sub(now)
	diff2 := nextTime2.Sub(now)
	diff3 := nextTime3.Sub(now)

	// They should all be approximately 720 minutes (180 * 4, multiplier caps at 4)
	for _, diff := range []time.Duration{diff1, diff2, diff3} {
		require.Greater(t, diff, 719*time.Minute+55*time.Second)
		require.Less(t, diff, 720*time.Minute+5*time.Second)
	}
}

func TestCalculateNextFetchTime_AlwaysReturnsValidTime(t *testing.T) {
	t.Parallel()

	nextTime := reader.CalculateNextFetchTime(0, 5.0, false)
	now := time.Now()

	// Should always return a future time
	require.True(t, nextTime.After(now))

	// Should be within reasonable bounds (between 1h and 24h)
	diff := nextTime.Sub(now)
	require.Greater(t, diff, reader.MinFetchInterval)
	require.Less(t, diff, reader.MaxFetchInterval+1*time.Second)
}
