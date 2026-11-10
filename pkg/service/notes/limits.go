package notes

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/can3p/pcom/pkg/service"
)

// sttWindow is the span the per-user and the site's minutes are counted over.
const sttWindow = 24 * time.Hour

// What the actor reads when the site's cap stops a transcription.
const msgSTTSiteCap = "Transcription is not available right now."

// Allow refuses a transcription of seconds of audio for the user when it
// would take them over their minutes for the last day, or the site over its
// daily cap. Only ok calls count: a failed one is not charged. It returns
// service.Invalid, whose message the actor reads.
func (s *Service) Allow(ctx context.Context, userID string, seconds int) error {
	now := s.now().UTC()

	used, err := s.store.STTSecondsSince(ctx, userID, now.Add(-sttWindow))
	if err != nil {
		return err
	}

	if limit := s.limits.STTMinutesPerDay * 60; used.Seconds+seconds > limit {
		return service.Invalid("", fmt.Sprintf("You have used %s of your %d minutes of transcription for today. More is available in %s.",
			wholeMinutes(used.Seconds), s.limits.STTMinutesPerDay, waitMinutes(used.Oldest.Add(sttWindow).Sub(now))))
	}

	if s.limits.STTSiteMinutesPerDay <= 0 {
		return nil
	}

	site, err := s.store.STTSecondsSince(ctx, "", now.Add(-sttWindow))
	if err != nil {
		return err
	}

	if site.Seconds+seconds > s.limits.STTSiteMinutesPerDay*60 {
		return service.Invalid("", msgSTTSiteCap)
	}

	return nil
}

// wholeMinutes is seconds as the minutes a sentence says, rounded up.
func wholeMinutes(seconds int) string {
	return fmt.Sprintf("%d", int(math.Ceil(float64(seconds)/60)))
}

// waitMinutes is a wait in whole minutes, rounded up.
func waitMinutes(d time.Duration) string {
	m := max(int(math.Ceil(d.Minutes())), 1)
	if m == 1 {
		return "1 minute"
	}

	return fmt.Sprintf("%d minutes", m)
}
