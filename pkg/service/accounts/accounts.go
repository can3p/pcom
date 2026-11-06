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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Service struct {
	store *repo.Store
	// jobs enqueues background jobs; see jobs.go.
	jobs  repo.JobQueue
	feeds *feeds.Service
	ident mail.Identity

	aboutMaxLength  int
	stylesMaxLength int
	// codeKey keys the HMAC of login codes; without it no code is issued
	// or checked.
	codeKey []byte
	now     func() time.Time
	login   LoginLimits
	logger  *zap.Logger
}

// New builds the service. The mail queue parameter is unused: mail goes out
// as jobs on the queue WithJobs sets. feeds may be nil when the settings page is not used.
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

// WithLogger sets the logger of the background pruner (RunPruner); requests
// log through the logger their context carries. The default logs nothing.
func WithLogger(l *zap.Logger) Option {
	return func(s *Service) {
		if l != nil {
			s.logger = l
		}
	}
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

func New(store *repo.Store, subscriptions *feeds.Service, opts ...Option) *Service {
	s := &Service{store: store, feeds: subscriptions, aboutMaxLength: DefaultProfileAboutMaxLength, stylesMaxLength: DefaultUserStylesMaxLength, now: time.Now, login: DefaultLoginLimits, logger: zap.NewNop()}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// SendAdminMail queues the page failure report for the site admin outside of
// any change: report is the request, the panic and its stack, and user the
// visitor who hit the page, nil when not logged in.
func (s *Service) SendAdminMail(ctx context.Context, report string, user *model.User) error {
	args := AdminPageFailureMailArgs{Report: report}
	if user != nil {
		args.UserID = user.ID
	}

	return s.store.Enqueue(ctx, s.jobs, args)
}

// UserByID loads the user a session belongs to.
func (s *Service) UserByID(ctx context.Context, id string) (*model.User, error) {
	return notFound(s.store.UserByID(ctx, id))
}

// UserByAPIKey loads the user an API key belongs to.
func (s *Service) UserByAPIKey(ctx context.Context, key string) (*model.User, error) {
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

// TextLimits are the most characters the texts this service stores may
// have. Forms read them to check their fields; the service enforces them on
// save.
type TextLimits struct {
	ProfileAboutMaxLength int
	UserStylesMaxLength   int
}

// TextLimits returns the service's text length limits.
func (s *Service) TextLimits() TextLimits {
	return TextLimits{ProfileAboutMaxLength: s.aboutMaxLength, UserStylesMaxLength: s.stylesMaxLength}
}

// SaveUserStyles replaces the actor's custom CSS. Empty styles remove it.
func (s *Service) SaveUserStyles(ctx context.Context, actor *model.User, styles string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if utf8.RuneCountInString(strings.TrimSpace(styles)) > s.stylesMaxLength {
		return service.Invalid("styles", fmt.Sprintf("The styles can have at most %d characters.", s.stylesMaxLength))
	}

	return s.store.SaveUserStyle(ctx, actor.ID, styles)
}

// SaveProfile replaces the actor's "About" text. An empty text, or one of
// whitespace only, removes it.
func (s *Service) SaveProfile(ctx context.Context, actor *model.User, about string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	about = strings.TrimSpace(about)
	if utf8.RuneCountInString(about) > s.aboutMaxLength {
		return service.Invalid("about", fmt.Sprintf("The About text can have at most %d characters.", s.aboutMaxLength))
	}

	if about == "" {
		return s.store.DeleteProfileAbout(ctx, actor.ID)
	}

	return s.store.SaveProfileAbout(ctx, actor.ID, about)
}

// SaveGeneralSettings changes the actor's timezone, who can read their
// journal and whether people without an account may subscribe to it by
// email. Subscriptions need a journal open to everybody, so enabling them
// under any other visibility is refused.
func (s *Service) SaveGeneralSettings(ctx context.Context, actor *model.User, timezone string, visibility model.ProfileVisibility, emailSubscriptions bool) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if emailSubscriptions && visibility != model.ProfileVisibilityPublic {
		return service.Invalid("email_subscriptions_enabled", "Email subscriptions need a journal that anyone on the internet can read.")
	}

	actor.Timezone = timezone
	actor.ProfileVisibility = visibility
	actor.EmailSubscriptionsEnabled = emailSubscriptions

	return s.store.SaveUser(ctx, actor,
		model.UserColumns.Timezone,
		model.UserColumns.ProfileVisibility,
		model.UserColumns.EmailSubscriptionsEnabled,
		model.UserColumns.UpdatedAt,
	)
}

// GenerateAPIKey gives the actor an API key. There is no rotation: a user
// who has a key keeps it.
func (s *Service) GenerateAPIKey(ctx context.Context, actor *model.User) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.CreateAPIKey(ctx, actor.ID)
	})
}

// RegenerateFeedToken creates the actor's private feed token or replaces it;
// the old feed URL stops working.
func (s *Service) RegenerateFeedToken(ctx context.Context, actor *model.User) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	_, err := s.store.RegenerateFeedToken(ctx, actor.ID)

	return err
}

// SettingsView is what the settings page shows besides the forms.
type SettingsView struct {
	AvailableInvites int64
	UsedInvites      []*model.UserInvitation
	APIKey           *model.UserAPIKey
	FeedURL          string // private RSS feed URL, empty until a feed token exists
	UserStyles       string
	ProfileAbout     string
	Feeds            []*feeds.RssFeed
}

// Settings gathers the actor's settings page.
func (s *Service) Settings(ctx context.Context, actor *model.User) (*SettingsView, error) {
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
func (s *Service) notifyAdmin(ctx context.Context, what string, args repo.JobArgs) {
	if err := s.store.Enqueue(ctx, s.jobs, args); err != nil {
		logging.FromContext(ctx).Warn("failed to queue the admin notification", zap.String("notification", what), zap.Error(err))
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
			if err := tx.InsertInvitation(ctx, &model.UserInvitation{ID: uuid.NewString(), UserID: u.ID}); err != nil {
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
