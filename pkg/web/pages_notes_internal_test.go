package web

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestStackLabel_DayPartInTheViewersZone(t *testing.T) {
	t.Parallel()

	utc := &model.User{Timezone: "UTC"}
	tokyo := &model.User{Timezone: "Asia/Tokyo"}
	at := func(h, m int) time.Time { return time.Date(2026, 11, 3, h, m, 0, 0, time.UTC) } // a Tuesday

	tests := []struct {
		name string
		user *model.User
		t    time.Time
		size int
		want string
	}{
		{"night before five", utc, at(4, 59), 2, "Tuesday night, 2 notes"},
		{"morning from five", utc, at(5, 0), 3, "Tuesday morning, 3 notes"},
		{"morning until noon", utc, at(11, 59), 3, "Tuesday morning, 3 notes"},
		{"afternoon from noon", utc, at(12, 0), 4, "Tuesday afternoon, 4 notes"},
		{"afternoon until five", utc, at(16, 59), 4, "Tuesday afternoon, 4 notes"},
		{"evening from five", utc, at(17, 0), 7, "Tuesday evening, 7 notes"},
		{"evening until ten", utc, at(21, 59), 7, "Tuesday evening, 7 notes"},
		{"night from ten", utc, at(22, 0), 2, "Tuesday night, 2 notes"},
		{"the viewer's zone moves the day", tokyo, at(20, 0), 2, "Wednesday morning, 2 notes"},
		{"no zone known is UTC", &model.User{}, at(13, 0), 2, "Tuesday afternoon, 2 notes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, stackLabel(tt.t, tt.user, tt.size))
		})
	}
}
