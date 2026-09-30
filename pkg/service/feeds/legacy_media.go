package feeds

import (
	"context"
	"io"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/repo"
)

// uploadFeedImage stores an image of a feed item. media.HandleUpload still
// takes an executor until L6 converts it; this is the only such call here.
func (s *Service) uploadFeedImage(ctx context.Context, tx *repo.Store, feedID string, r io.Reader) (string, error) {
	return media.HandleUpload(ctx, tx.Exec(), s.mediaStorage, nil, &feedID, r)
}
