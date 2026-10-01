// Package posts is everything users do with posts: write, edit, autosave,
// publish and delete them, comment on them, prompt others to write, export
// and import a blog, and the API v1 that does the same for scripts.
package posts

import (
	"context"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/pkg/errors"
)

type Service struct {
	store   *repo.Store
	sender  repo.MailQueue
	storage server.MediaStorage
	ident   mail.Identity

	translations Translations
}

// Translations is what posts needs from the translation service: the
// translations of a post are refreshed when its text changes and dropped when
// its author stops allowing translation. Both run in the save transaction.
type Translations interface {
	// Enabled reports whether a translation provider is configured.
	Enabled() bool
	RetranslateStale(ctx context.Context, tx *repo.Store, postID string) error
	Forget(ctx context.Context, tx *repo.Store, postID string) error
}

// Option changes how New builds the service.
type Option func(*Service)

// WithIdentity tells the service the site its mail links point to and the
// address the mail comes from.
func WithIdentity(ident mail.Identity) Option {
	return func(s *Service) { s.ident = ident }
}

// WithTranslations makes saves keep the post's translations in step. Without
// it nothing is translated and the editor offers no toggle.
func WithTranslations(t Translations) Option {
	return func(s *Service) { s.translations = t }
}

// TranslationEnabled reports whether authors can allow translation of their
// posts.
func (s *Service) TranslationEnabled() bool {
	return s.translations != nil && s.translations.Enabled()
}

func New(store *repo.Store, snd repo.MailQueue, storage server.MediaStorage, opts ...Option) *Service {
	s := &Service{store: store, sender: snd, storage: storage}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// PostURL is the public address of a post.
func (s *Service) PostURL(postID string) string {
	return s.ident.Site.Abs("post", postID)
}

// queue puts a formatted mail in the outbox of the transaction tx. A nil mail
// is a notification nobody needs, and is skipped.
func (s *Service) queue(ctx context.Context, tx *repo.Store, out *mail.Outgoing) error {
	if out == nil {
		return nil
	}

	if err := tx.SendMail(ctx, s.sender, out.UniqueID, out.Type, out.Mail); err != nil {
		return errors.Wrap(err, "failed to queue email")
	}

	return nil
}

// queueE is queue for the formatters that can fail.
func (s *Service) queueE(ctx context.Context, tx *repo.Store, out *mail.Outgoing, err error) error {
	if err != nil {
		return err
	}

	return s.queue(ctx, tx, out)
}

// requireActor turns an anonymous actor into ErrNeedsLogin.
func requireActor(actor *core.User) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return nil
}
