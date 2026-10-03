// Package accounts holds everything about who a user is and how they get in:
// signup, the waiting list, invitations, login credentials, passwords, the
// settings page, user styles, API keys, feed tokens and the registration
// switch.
package accounts

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/can3p/pcom/pkg/forms/validation"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/google/uuid"
)

type Service struct {
	store  *repo.Store
	sender repo.MailQueue
	feeds  *feeds.Service
	ident  mail.Identity

	aboutMaxLength  int
	stylesMaxLength int
	// codeKey keys the HMAC of login codes; without it no code is issued
	// or checked.
	codeKey []byte
	now     func() time.Time
	login   LoginLimits
}

// New builds the service. sender may be nil for a command line script that
// sends no mail; feeds may be nil when the settings page is not used.
// Option changes how New builds the service.
type Option func(*Service)

// DefaultProfileAboutMaxLength is the About text limit when the
// configuration sets none; it is also the configuration's default.
const DefaultProfileAboutMaxLength = 6_000

// WithProfileAboutMaxLength sets the most characters the About text may
// have; zero keeps DefaultProfileAboutMaxLength.
func WithProfileAboutMaxLength(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.aboutMaxLength = n
		}
	}
}

// DefaultUserStylesMaxLength is the custom CSS limit when the configuration
// sets none; it is also the configuration's default.
const DefaultUserStylesMaxLength = 10_000

// WithUserStylesMaxLength sets the most characters the custom CSS may have;
// zero keeps DefaultUserStylesMaxLength.
func WithUserStylesMaxLength(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.stylesMaxLength = n
		}
	}
}

// WithIdentity tells the service the site its links point to and the
// addresses its mail uses. Without it links are relative and mail has no sender.
func WithIdentity(ident mail.Identity) Option {
	return func(s *Service) { s.ident = ident }
}

// WithCodeKey sets the key login codes are hashed with. The server and any
// command that issues codes must use the same key.
func WithCodeKey(key string) Option {
	return func(s *Service) { s.codeKey = []byte(key) }
}

// WithClock replaces the clock login attempts are timed by, for tests.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLoginLimits sets the limits of login by code; a zero field keeps its
// DefaultLoginLimits value.
func WithLoginLimits(l LoginLimits) Option {
	return func(s *Service) {
		d := DefaultLoginLimits
		s.login = LoginLimits{
			CodeLifetime:        cmp.Or(l.CodeLifetime, d.CodeLifetime),
			CodeTries:           cmp.Or(l.CodeTries, d.CodeTries),
			CodesMailed:         cmp.Or(l.CodesMailed, d.CodesMailed),
			CodesMailedWindow:   cmp.Or(l.CodesMailedWindow, d.CodesMailedWindow),
			WrongTries:          cmp.Or(l.WrongTries, d.WrongTries),
			WrongTriesWindow:    cmp.Or(l.WrongTriesWindow, d.WrongTriesWindow),
			UnconfirmedLifetime: cmp.Or(l.UnconfirmedLifetime, d.UnconfirmedLifetime),
			PruneEvery:          cmp.Or(l.PruneEvery, d.PruneEvery),
		}
	}
}

func New(store *repo.Store, snd repo.MailQueue, subscriptions *feeds.Service, opts ...Option) *Service {
	s := &Service{store: store, sender: snd, feeds: subscriptions, aboutMaxLength: DefaultProfileAboutMaxLength, stylesMaxLength: DefaultUserStylesMaxLength, now: time.Now, login: DefaultLoginLimits}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// send queues a mail on the store, so inside Tx it goes out only if the
// change commits.
func (s *Service) send(ctx context.Context, store *repo.Store, e *mail.Envelope) error {
	return store.SendMail(ctx, s.sender, e.UniqueID, e.Type, e.Mail)
}

// SendAdminMail queues a notification for the site admin outside of any
// change, for the page failure report.
func (s *Service) SendAdminMail(ctx context.Context, e *mail.Envelope) error {
	return s.send(ctx, s.store, e)
}

// UserByID loads the user a session belongs to.
func (s *Service) UserByID(ctx context.Context, id string) (*core.User, error) {
	return notFound(s.store.UserByID(ctx, id))
}

// UserByAPIKey loads the user an API key belongs to.
func (s *Service) UserByAPIKey(ctx context.Context, key string) (*core.User, error) {
	return notFound(s.store.UserByAPIKey(ctx, key))
}

// RegistrationOpen reports whether anybody may sign up.
func (s *Service) RegistrationOpen(ctx context.Context) (bool, error) {
	return s.store.RegistrationOpen(ctx)
}

// SetRegistrationOpen opens or closes registration.
func (s *Service) SetRegistrationOpen(ctx context.Context, open bool) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.SetRegistrationOpen(ctx, open)
	})
}

// UserStyles returns the custom CSS the user has saved, trimmed, or "" when
// the user or the styles do not exist.
func (s *Service) UserStyles(ctx context.Context, username string) (string, error) {
	style, err := s.store.UserStyleByUsername(ctx, username)
	if err != nil || style == nil {
		return "", err
	}

	return strings.TrimSpace(style.Styles), nil
}

// SaveUserStyles replaces the actor's custom CSS.
func (s *Service) SaveUserStyles(ctx context.Context, actor *core.User, styles string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if err := s.ValidateUserStyles(styles); err != nil {
		return service.Invalid("styles", err.Error())
	}

	return s.store.SaveUserStyle(ctx, actor.ID, styles)
}

// ValidateUserStyles checks the length of the custom CSS. Empty styles are
// valid: they remove the custom CSS.
func (s *Service) ValidateUserStyles(styles string) error {
	return validation.ValidateMinMax("styles", styles, 0, s.stylesMaxLength)
}

// ValidateProfileAbout checks the length of the "About" text. An empty text
// is valid: it removes the section.
func (s *Service) ValidateProfileAbout(about string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(about)); n > s.aboutMaxLength {
		return fmt.Errorf("about text can have at most %d characters, this one has %d", s.aboutMaxLength, n)
	}

	return nil
}

// SaveProfile replaces the actor's "About" text. An empty text, or one of
// whitespace only, removes it.
func (s *Service) SaveProfile(ctx context.Context, actor *core.User, about string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if err := s.ValidateProfileAbout(about); err != nil {
		return service.Invalid("about", fmt.Sprintf("The About text can have at most %d characters.", s.aboutMaxLength))
	}

	about = strings.TrimSpace(about)
	if about == "" {
		return s.store.DeleteProfileAbout(ctx, actor.ID)
	}

	return s.store.SaveProfileAbout(ctx, actor.ID, about)
}

// SaveGeneralSettings changes the actor's timezone and profile visibility.
func (s *Service) SaveGeneralSettings(ctx context.Context, actor *core.User, timezone string, visibility core.ProfileVisibility) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	actor.Timezone = timezone
	actor.ProfileVisibility = visibility

	return s.store.SaveUser(ctx, actor,
		core.UserColumns.Timezone,
		core.UserColumns.ProfileVisibility,
		core.UserColumns.UpdatedAt,
	)
}

// GenerateAPIKey gives the actor an API key. There is no rotation: a user
// who has a key keeps it.
func (s *Service) GenerateAPIKey(ctx context.Context, actor *core.User) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.CreateAPIKey(ctx, actor.ID)
	})
}

// RegenerateFeedToken creates the actor's private feed token or replaces it;
// the old feed URL stops working.
func (s *Service) RegenerateFeedToken(ctx context.Context, actor *core.User) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	_, err := s.store.RegenerateFeedToken(ctx, actor.ID)

	return err
}

// SettingsView is what the settings page shows besides the forms.
type SettingsView struct {
	AvailableInvites int64
	UsedInvites      core.UserInvitationSlice
	APIKey           *core.UserAPIKey
	FeedURL          string // private RSS feed URL, empty until a feed token exists
	UserStyles       string
	ProfileAbout     string
	Feeds            []*feeds.RssFeed
}

// Settings gathers the actor's settings page.
func (s *Service) Settings(ctx context.Context, actor *core.User) (*SettingsView, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	totalInvites, err := s.store.InvitationCount(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	usedInvites, err := s.store.SentInvitations(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	apiKey, err := s.store.APIKeyForUser(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	feedToken, err := s.store.FeedTokenForUser(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	view := &SettingsView{
		AvailableInvites: totalInvites - int64(len(usedInvites)),
		UsedInvites:      usedInvites,
		APIKey:           apiKey,
	}

	if feedToken != nil {
		view.FeedURL = s.ident.Site.Abs("private_user_feed", feedToken.Token)
	}

	style, err := s.store.UserStyleForUser(ctx, actor.ID)
	if err != nil {
		return nil, err
	} else if style != nil {
		view.UserStyles = style.Styles
	}

	if view.ProfileAbout, err = s.store.ProfileAbout(ctx, actor.ID); err != nil {
		return nil, err
	}

	if s.feeds != nil {
		if view.Feeds, err = s.feeds.Subscriptions(ctx, actor); err != nil {
			return nil, err
		}
	}

	return view, nil
}

// notFound maps the repository's not found to the service error.
func notFound[T any](v T, err error) (T, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return v, service.ErrNotFound
	}

	return v, err
}

// notifyAdmin queues an admin notification after the change it reports
// committed; a failure is logged, never returned.
func (s *Service) notifyAdmin(ctx context.Context, what string, e *mail.Envelope) {
	if err := s.send(ctx, s.store, e); err != nil {
		log.Printf("failed to queue the %s notification: %v", what, err)
	}
}

// AddInvites gives the user with the email n more invitations (for a
// command line script).
func (s *Service) AddInvites(ctx context.Context, email string, n int) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		u, err := notFound(tx.UserByEmail(ctx, pgsession.NormalizeEmail(email)))
		if err != nil {
			return fmt.Errorf("user [%s]: %w", email, err)
		}

		for ; n > 0; n-- {
			if err := tx.InsertInvitation(ctx, &core.UserInvitation{ID: uuid.NewString(), UserID: u.ID}); err != nil {
				return err
			}
		}

		return nil
	})
}

// FatalError marks a failure the transports treat as a bug, not as something
// to show the user: a confirmation mail that cannot be queued, or the waiting
// list write. They panic on it, as they always did, so that the recovery
// middleware answers 500 and mails the admin.
type FatalError struct{ Err error }

func (e *FatalError) Error() string { return e.Err.Error() }
func (e *FatalError) Unwrap() error { return e.Err }

func fatal(err error) error {
	if err == nil {
		return nil
	}

	return &FatalError{Err: err}
}
