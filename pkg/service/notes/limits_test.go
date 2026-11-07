package notes_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestAllow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	now := time.Now().UTC()

	me := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)

	record := func(userID string, seconds int, status model.STTStatus, ago time.Duration) {
		t.Helper()

		require.NoError(t, store.InsertSTTRequest(ctx, &model.STTRequest{
			UserID: userID, Provider: "p", Model: "m", Seconds: seconds, Status: status, CreatedAt: now.Add(-ago),
		}))
	}

	// me: 8 minutes ok an hour ago, a failed call and a day-old one that count for nothing.
	record(me.ID, 8*60, model.STTStatusOK, time.Hour)
	record(me.ID, 60*60, model.STTStatusFailed, time.Minute)
	record(me.ID, 60*60, model.STTStatusOK, 25*time.Hour)
	// The window's edge: a minute 23h59m ago counts, one 24h01m ago does not.
	record(me.ID, 60, model.STTStatusOK, 23*time.Hour+59*time.Minute)
	record(me.ID, 60, model.STTStatusOK, 24*time.Hour+time.Minute)
	// other: 5 minutes ok, which only the site's cap sees.
	record(other.ID, 5*60, model.STTStatusOK, time.Minute)

	svc := func(site int) *notes.Service {
		return notes.New(store, notes.WithClock(func() time.Time { return now }),
			notes.WithLimits(notes.Limits{STTMinutesPerDay: 10, STTSiteMinutesPerDay: site}))
	}

	cases := []struct {
		name    string
		userID  string
		svc     *notes.Service
		seconds int
		msg     string
	}{
		{"within the user's minutes", me.ID, svc(0), 60, ""},
		{"over the user's minutes", me.ID, svc(0), 61,
			"You have used 9 of your 10 minutes of transcription for today. More is available in 1 minute."},
		{"within the site's cap", other.ID, svc(15), 60, ""},
		{"over the site's cap", other.ID, svc(15), 2 * 60, "Transcription is not available right now."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.svc.Allow(ctx, tc.userID, tc.seconds)
			if tc.msg == "" {
				require.NoError(t, err)
				return
			}

			var verr *service.ValidationError

			require.ErrorAs(t, err, &verr)
			require.Equal(t, tc.msg, verr.Message)
		})
	}
}
