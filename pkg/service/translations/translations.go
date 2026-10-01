// Package translations translates posts and RSS items into English for
// readers: it caches translations, keeps the per-reader daily and site-wide
// monthly character budgets, and re-translates edited posts in the
// background through a queue its worker sends, as mail is sent.
package translations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/translate"
)

// ErrNotImplemented is returned by the methods T2 has yet to fill.
var ErrNotImplemented = errors.New("translations: not implemented")

// DefaultPollInterval is how often the worker looks for due jobs.
const DefaultPollInterval = 30 * time.Second

// Limits are the character budgets: a reader's per day and the site's per
// calendar month. Background re-translations count only against the site's.
type Limits struct {
	UserDailyChars   int
	SiteMonthlyChars int
}

// Service translates. Its translator is nil when translation is off.
type Service struct {
	store      *repo.Store
	translator *translate.Translator
	limits     Limits
}

// New returns the service; translator nil turns translation off.
func New(store *repo.Store, translator *translate.Translator, limits Limits) *Service {
	return &Service{store: store, translator: translator, limits: limits}
}

// Enabled reports whether a backend is configured.
func (s *Service) Enabled() bool { return s.translator != nil }

// Result is a translation shown to a reader.
type Result struct {
	Subject string
	Body    string
	// SourceLang is the source's language, ISO 639-1; empty if unknown.
	SourceLang string
	// Provider is the backend's DisplayName, such as "Azure Translator".
	Provider string
}

// Translate returns the English translation of a post or an RSS item the
// actor may read, from the cache or the backend.
func (s *Service) Translate(ctx context.Context, actor *core.User, kind core.TranslationSourceKind, id string) (*Result, error) {
	return nil, fmt.Errorf("%w: Translate %s %s", ErrNotImplemented, kind, id)
}

// RetranslateStale queues re-translations of the post's cached translations
// on tx, after an edit.
func (s *Service) RetranslateStale(ctx context.Context, tx *repo.Store, postID string) error {
	return fmt.Errorf("%w: RetranslateStale %s", ErrNotImplemented, postID)
}

// Forget drops the post's cached translations and queued jobs on tx, such as
// when its author stops allowing translation.
func (s *Service) Forget(ctx context.Context, tx *repo.Store, postID string) error {
	return fmt.Errorf("%w: Forget %s", ErrNotImplemented, postID)
}

// Run sends the due re-translation jobs every interval until ctx is done.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.runJobs(ctx); err != nil {
				slog.Warn("Failed to run translation jobs", "err", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

// runJobs sends the due jobs; a panic in a backend doesn't stop the worker.
func (s *Service) runJobs(ctx context.Context) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("translation jobs panicked: %v", p)
		}
	}()

	if !s.Enabled() {
		return nil
	}

	return nil // T2 sends the queue here
}
