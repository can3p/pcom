package proofread

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
)

// The windows of the per-user and the site's limits.
const (
	userWindow = time.Hour
	siteWindow = 24 * time.Hour
)

// What the actor reads when a limit stops a call.
const (
	msgSiteCap = "Grammar checks and style reviews are not available right now."
)

// perHour is the hourly limit of a kind of call, and what the limit message
// calls such calls.
func (s *Service) perHour(kind model.ProofreadKind) (int, string) {
	if kind == model.ProofreadKindStyle {
		return s.limits.StylePerHour, "style reviews"
	}

	return s.limits.GrammarPerHour, "grammar checks"
}

// used counts the actor's ok calls of a kind in the hour before now.
func (s *Service) used(ctx context.Context, userID string, kind model.ProofreadKind, now time.Time) (repo.ProofreadRequestCount, error) {
	return s.store.ProofreadRequestsSince(ctx, repo.ProofreadRequestFilter{
		UserID: userID,
		Kind:   kind,
		Status: model.ProofreadStatusOK,
		Since:  now.Add(-userWindow),
	})
}

// checkLimits refuses a call of kind when the actor has used up the hour's
// limit or the site its daily cap. Failed calls count against neither.
func (s *Service) checkLimits(ctx context.Context, actor *model.User, kind model.ProofreadKind) error {
	now := s.now().UTC()
	limit, noun := s.perHour(kind)

	u, err := s.used(ctx, actor.ID, kind, now)
	if err != nil {
		return err
	}

	if u.Count >= limit {
		return service.Invalid("", fmt.Sprintf("You have used all %d %s for this hour. The next one is available in %s.",
			limit, noun, minutes(u.Oldest.Add(userWindow).Sub(now))))
	}

	if s.limits.SiteDailyRequests <= 0 {
		return nil
	}

	site, err := s.store.ProofreadRequestsSince(ctx, repo.ProofreadRequestFilter{
		Status: model.ProofreadStatusOK,
		Since:  now.Add(-siteWindow),
	})
	if err != nil {
		return err
	}

	if site.Count >= s.limits.SiteDailyRequests {
		return service.Invalid("", msgSiteCap)
	}

	return nil
}

// minutes is a wait in whole minutes, rounded up, as a sentence says it.
func minutes(d time.Duration) string {
	m := max(int(math.Ceil(d.Minutes())), 1)
	if m == 1 {
		return "1 minute"
	}

	return fmt.Sprintf("%d minutes", m)
}

// remaining is what is left of the actor's hourly limits.
func (s *Service) remaining(ctx context.Context, userID string) (Remaining, error) {
	now := s.now().UTC()

	g, err := s.used(ctx, userID, model.ProofreadKindGrammar, now)
	if err != nil {
		return Remaining{}, err
	}

	st, err := s.used(ctx, userID, model.ProofreadKindStyle, now)
	if err != nil {
		return Remaining{}, err
	}

	return Remaining{
		Grammar:      max(s.limits.GrammarPerHour-g.Count, 0),
		Style:        max(s.limits.StylePerHour-st.Count, 0),
		GrammarLimit: s.limits.GrammarPerHour,
		StyleLimit:   s.limits.StylePerHour,
	}, nil
}

// Remaining returns what is left of the actor's hourly limits.
func (s *Service) Remaining(ctx context.Context, actor *model.User) (Remaining, error) {
	if err := s.begin(actor); err != nil {
		return Remaining{}, err
	}

	return s.remaining(ctx, actor.ID)
}
