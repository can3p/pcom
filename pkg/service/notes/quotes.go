package notes

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
)

// QuoteSource is the source a new quote of the actor starts with: the source
// of their most recent note that has one, when that note was captured within
// the quote source window, else nil.
func (s *Service) QuoteSource(ctx context.Context, actor *model.User) (*model.NoteSource, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	n, err := s.store.LastSourcedNote(ctx, actor.ID)
	if err != nil || n == nil || n.Source == nil {
		return nil, err
	}

	if s.now().Sub(n.CapturedAt) > s.limits.QuoteSourceWindow {
		return nil, nil
	}

	return n.Source, nil
}
