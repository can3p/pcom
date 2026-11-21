package accounts

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
)

// NoticeScratchPad tells an author that a new post is kept as a scratch pad
// until it is saved as a draft.
const NoticeScratchPad = "scratch_pad"

// notices is every notice id a user may dismiss.
var notices = map[string]bool{NoticeScratchPad: true}

// NoticeShown reports whether the actor still has to be shown the notice:
// until they dismiss it, it is shown on every device. Anonymous visitors and
// unknown notices get none.
func (s *Service) NoticeShown(ctx context.Context, actor *model.User, notice string) (bool, error) {
	if actor == nil || !notices[notice] {
		return false, nil
	}

	dismissed, err := s.store.NoticeDismissed(ctx, actor.ID, notice)

	return !dismissed, err
}

// DismissNotice records that the actor dismissed the notice, for their
// account. Dismissing twice is fine.
func (s *Service) DismissNotice(ctx context.Context, actor *model.User, notice string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if !notices[notice] {
		return service.Invalid("notice", "Unknown notice.")
	}

	return s.store.DismissNotice(ctx, actor.ID, notice, s.now())
}
