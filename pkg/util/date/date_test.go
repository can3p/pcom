package date

import (
	"html/template"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/stretchr/testify/require"
)

func TestLocalizeTime(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		user     *core.User
		input    time.Time
		expected time.Time
	}{
		{
			name:     "nil user returns original time",
			user:     nil,
			input:    baseTime,
			expected: baseTime,
		},
		{
			name:     "user with UTC timezone",
			user:     &core.User{Timezone: "UTC"},
			input:    baseTime,
			expected: baseTime,
		},
		{
			name:     "user with America/New_York timezone",
			user:     &core.User{Timezone: "America/New_York"},
			input:    baseTime,
			expected: baseTime.In(mustLoadLocation("America/New_York")),
		},
		{
			name:     "user with invalid timezone returns original time",
			user:     &core.User{Timezone: "Invalid/Timezone"},
			input:    baseTime,
			expected: baseTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LocalizeTime(tt.user, tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		user     *core.User
		input    time.Time
		expected string
	}{
		{
			name:     "nil user formats in UTC",
			user:     nil,
			input:    baseTime,
			expected: "Mon, 15 Jan 2024 12:30",
		},
		{
			name:     "user with timezone formats in local time",
			user:     &core.User{Timezone: "America/New_York"},
			input:    baseTime,
			expected: "Mon, 15 Jan 2024 07:30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTimestamp(tt.input, tt.user)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)

	result := RelativeTime(now.Add(-5*time.Minute), now)
	require.Equal(t, "5 minutes ago", result)

	result = RelativeTime(now.Add(-2*time.Hour), now)
	require.Equal(t, "2 hours ago", result)
}

func TestRenderTimeHTML(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)
	baseTime := now.Add(-5 * time.Minute)

	result := RenderTimeHTML(baseTime, nil, now)

	expected := template.HTML(`<span title="Mon, 15 Jan 2024 12:25">5 minutes ago</span>`)
	require.Equal(t, expected, result)
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
