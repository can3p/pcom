package date

import (
	"html/template"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestLocalizeTime(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		user     *model.User
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
			user:     &model.User{Timezone: "UTC"},
			input:    baseTime,
			expected: baseTime,
		},
		{
			name:     "user with America/New_York timezone",
			user:     &model.User{Timezone: "America/New_York"},
			input:    baseTime,
			expected: baseTime.In(mustLoadLocation("America/New_York")),
		},
		{
			name:     "user with invalid timezone returns original time",
			user:     &model.User{Timezone: "Invalid/Timezone"},
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
		user     *model.User
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
			user:     &model.User{Timezone: "America/New_York"},
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

func TestRenderDateHTML(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	user := &model.User{Timezone: "America/New_York"}

	tests := []struct {
		name string
		t    time.Time
		want template.HTML
	}{
		{
			name: "within the recent window stays relative",
			t:    now.Add(-3 * time.Hour),
			want: `<time datetime="2024-06-15T09:00:00Z" title="Sat, 15 Jun 2024 05:00 EDT · 3 hours ago">3 hours ago</time>`,
		},
		{
			name: "this year shows day and month",
			t:    time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC),
			want: `<time datetime="2024-01-15T12:30:00Z" title="Mon, 15 Jan 2024 07:30 EST · 5 months ago">15 Jan</time>`,
		},
		{
			name: "earlier year shows the year",
			t:    time.Date(2023, 12, 31, 3, 0, 0, 0, time.UTC),
			want: `<time datetime="2023-12-31T03:00:00Z" title="Sat, 30 Dec 2023 22:00 EST · 5 months ago">30 Dec 2023</time>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderDateHTML(tt.t, user, now)
			require.Equal(t, tt.want, got)
		})
	}
}
