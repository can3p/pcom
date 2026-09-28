package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeZonesSanity(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, TimeZones, "TimeZones list should not be empty")

	// Check that every timezone in the list is valid and can be loaded
	for _, tz := range TimeZones {
		t.Run(tz, func(t *testing.T) {
			loc, err := time.LoadLocation(tz)
			require.NoError(t, err, "timezone %q should be loadable", tz)
			require.NotNil(t, loc)
		})
	}
}

func TestTimeZonesContainsCommon(t *testing.T) {
	t.Parallel()

	commonZones := []string{
		"America/New_York",
		"America/Los_Angeles",
		"America/Chicago",
		"Europe/London",
		"Europe/Paris",
		"Asia/Tokyo",
		"Asia/Shanghai",
		"Australia/Sydney",
		"Africa/Cairo",
	}

	tzMap := make(map[string]bool)
	for _, tz := range TimeZones {
		tzMap[tz] = true
	}

	for _, common := range commonZones {
		require.True(t, tzMap[common], "expected timezone %q in TimeZones list", common)
	}
}

func TestTimeZonesNoDuplicates(t *testing.T) {
	t.Parallel()

	seen := make(map[string]int)
	for _, tz := range TimeZones {
		seen[tz]++
	}

	for tz, count := range seen {
		require.Equal(t, 1, count, "timezone %q appears %d times, should appear exactly once", tz, count)
	}
}

func TestTimeZonesNoEmpty(t *testing.T) {
	t.Parallel()

	for i, tz := range TimeZones {
		require.NotEmpty(t, tz, "TimeZones[%d] should not be empty", i)
	}
}

func TestTimeZonesCanLocalize(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2024, 3, 15, 12, 30, 0, 0, time.UTC)

	for _, tzName := range TimeZones {
		t.Run(tzName, func(t *testing.T) {
			loc, err := time.LoadLocation(tzName)
			require.NoError(t, err)

			localTime := testTime.In(loc)
			require.Equal(t, tzName, localTime.Location().String())
		})
	}
}
