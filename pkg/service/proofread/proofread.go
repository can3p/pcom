// Package proofread is the grammar check and the style review of a post the
// author is writing: it enforces the per-user hourly limits and the site's
// daily cap, records every call to the model, keeps a post's style reviews
// and the status of their notes, and ties the model's answers to the text
// (pkg/proofread). Built with a nil Model the feature is off: Enabled reports
// false and every method answers service.ErrNotFound.
package proofread

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"go.uber.org/zap"
)

// Defaults of the limits and models, also the configuration's defaults.
const (
	DefaultGrammarPerHour    = 10
	DefaultStylePerHour      = 3
	DefaultSiteDailyRequests = 0 // no cap

	DefaultGrammarModel  = "claude-haiku-5-5"
	DefaultGrammarEffort = proofread.EffortLow
	DefaultStyleModel    = "claude-sonnet-5-5"
	DefaultStyleEffort   = proofread.EffortMedium
)

// Limits are how much of the feature users may use, and how long the text
// may be.
type Limits struct {
	// GrammarPerHour and StylePerHour are the ok calls of each kind one
	// user may make in an hour.
	GrammarPerHour, StylePerHour int
	// SiteDailyRequests is the most calls of both kinds the whole site may
	// make in a day; zero means no cap.
	SiteDailyRequests int
	// PostSubjectMaxLength and PostBodyMaxLength are the post form's limits;
	// longer input is rejected with the form's messages.
	PostSubjectMaxLength, PostBodyMaxLength int
}

// DefaultLimits are the limits of a service built without configuration.
var DefaultLimits = Limits{
	GrammarPerHour:       DefaultGrammarPerHour,
	StylePerHour:         DefaultStylePerHour,
	SiteDailyRequests:    DefaultSiteDailyRequests,
	PostSubjectMaxLength: posts.DefaultPostSubjectMaxLength,
	PostBodyMaxLength:    posts.DefaultPostBodyMaxLength,
}

// Models are the model and effort of each kind of call.
type Models struct {
	Grammar, Style proofread.ModelChoice
}

// DefaultModels are the models of a service built without configuration.
var DefaultModels = Models{
	Grammar: proofread.ModelChoice{Name: DefaultGrammarModel, Effort: DefaultGrammarEffort},
	Style:   proofread.ModelChoice{Name: DefaultStyleModel, Effort: DefaultStyleEffort},
}

// Option changes how New builds the service.
type Option func(*Service)

// WithLimits sets the limits; a zero field keeps its default (for
// SiteDailyRequests the default is no cap).
func WithLimits(l Limits) Option {
	return func(s *Service) {
		if l.GrammarPerHour > 0 {
			s.limits.GrammarPerHour = l.GrammarPerHour
		}

		if l.StylePerHour > 0 {
			s.limits.StylePerHour = l.StylePerHour
		}

		if l.SiteDailyRequests > 0 {
			s.limits.SiteDailyRequests = l.SiteDailyRequests
		}

		if l.PostSubjectMaxLength > 0 {
			s.limits.PostSubjectMaxLength = l.PostSubjectMaxLength
		}

		if l.PostBodyMaxLength > 0 {
			s.limits.PostBodyMaxLength = l.PostBodyMaxLength
		}
	}
}

// WithModels sets the model names and efforts; an empty name or effort keeps
// its default.
func WithModels(m Models) Option {
	return func(s *Service) {
		s.models.Grammar = choose(s.models.Grammar, m.Grammar)
		s.models.Style = choose(s.models.Style, m.Style)
	}
}

func choose(def, set proofread.ModelChoice) proofread.ModelChoice {
	if set.Name != "" {
		def.Name = set.Name
	}

	if set.Effort != "" {
		def.Effort = set.Effort
	}

	return def
}

// WithLogger sets the logger used when a call's context carries none
// (requests log through the logger their context carries). The default logs
// nothing.
func WithLogger(l *zap.Logger) Option {
	return func(s *Service) {
		if l != nil {
			s.logger = l
		}
	}
}

// Service is the proofread service. Build it with New.
type Service struct {
	store  *repo.Store
	model  proofread.Model
	limits Limits
	models Models
	logger *zap.Logger
}

// New builds the service. A nil m turns the feature off.
func New(store *repo.Store, m proofread.Model, opts ...Option) *Service {
	s := &Service{store: store, model: m, limits: DefaultLimits, models: DefaultModels, logger: zap.NewNop()}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Enabled reports whether the feature is on: pages show its buttons and
// panel, and routes answer, only when it is.
func (s *Service) Enabled() bool { return s.model != nil }

// Limits returns the service's limits.
func (s *Service) Limits() Limits { return s.limits }

// Models returns the models and efforts the service asks.
func (s *Service) Models() Models { return s.models }

// GrammarRequest is the text of a grammar check. PostID is the post being
// edited, or empty for one never saved.
type GrammarRequest struct {
	PostID  string
	Subject string
	Body    string
}

// GrammarResult is a grammar check's answer: the suggestions that were found
// in the text, in its order, and what is left of the limits.
type GrammarResult struct {
	Suggestions []proofread.Suggestion
	Remaining   Remaining
}

// StyleRequest is the text of a style review of a saved post, and what the
// author wants the reviewer to know.
type StyleRequest struct {
	PostID     string
	Subject    string
	Body       string
	AuthorNote string
}

// StyleReview is a new style review with its notes, and what is left of the
// limits.
type StyleReview struct {
	Review    *model.StyleReview
	Remaining Remaining
}

// StyleState is a post's style reviews: the latest with its notes (nil when
// there is none), the earlier ones newest first, and what is left of the
// limits.
type StyleState struct {
	Latest    *model.StyleReview
	Earlier   []*model.StyleReview
	Remaining Remaining
}

// Remaining is what is left of a user's hourly limits.
type Remaining struct {
	Grammar, GrammarLimit int
	Style, StyleLimit     int
}

// errNotImplemented is returned until the methods are written.
var errNotImplemented = errors.New("proofread: not implemented")

// CheckGrammar asks the model for a grammar check of the actor's text.
func (s *Service) CheckGrammar(ctx context.Context, actor *model.User, req GrammarRequest) (*GrammarResult, error) {
	return nil, errNotImplemented
}

// ReviewStyle asks the model for a style review of the actor's post and
// stores it, settling the open notes of the previous review.
func (s *Service) ReviewStyle(ctx context.Context, actor *model.User, req StyleRequest) (*StyleReview, error) {
	return nil, errNotImplemented
}

// StyleState returns the style reviews of the actor's post.
func (s *Service) StyleState(ctx context.Context, actor *model.User, postID string) (*StyleState, error) {
	return nil, errNotImplemented
}

// SetNoteStatus marks a note of the actor's latest review done, kept (with
// reason) or open again.
func (s *Service) SetNoteStatus(ctx context.Context, actor *model.User, noteID string, status model.StyleNoteStatus, reason string) error {
	return errNotImplemented
}

// ResetStyle deletes every style review of the actor's post.
func (s *Service) ResetStyle(ctx context.Context, actor *model.User, postID string) error {
	return errNotImplemented
}

// Remaining returns what is left of the actor's hourly limits.
func (s *Service) Remaining(ctx context.Context, actor *model.User) (Remaining, error) {
	return Remaining{}, errNotImplemented
}
