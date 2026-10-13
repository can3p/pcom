// Package proofread is the grammar check and the style review of a post the
// author is writing: it enforces the per-user hourly limits and the site's
// daily cap, records every call to the model, keeps a post's style reviews
// and the status of their notes, and ties the model's answers to the text
// (pkg/proofread). Built with a nil Model the feature is off: Enabled reports
// false and every method answers service.ErrNotFound.
package proofread

import (
	"time"

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
	DefaultNoteMaxLength     = 2000

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
	// NoteMaxLength is the longest author note to a style review, and the
	// longest reason to keep a note as is.
	NoteMaxLength int
}

// DefaultLimits are the limits of a service built without configuration.
var DefaultLimits = Limits{
	GrammarPerHour:       DefaultGrammarPerHour,
	StylePerHour:         DefaultStylePerHour,
	SiteDailyRequests:    DefaultSiteDailyRequests,
	PostSubjectMaxLength: posts.DefaultPostSubjectMaxLength,
	PostBodyMaxLength:    posts.DefaultPostBodyMaxLength,
	NoteMaxLength:        DefaultNoteMaxLength,
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

		if l.NoteMaxLength > 0 {
			s.limits.NoteMaxLength = l.NoteMaxLength
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
			s.log = l
		}
	}
}

// WithClock replaces the clock the hourly and daily windows are timed by,
// for tests.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// Service is the proofread service. Build it with New.
type Service struct {
	store  *repo.Store
	posts  *posts.Service
	model  proofread.Model
	limits Limits
	models Models
	log    *zap.Logger
	now    func() time.Time
}

// New builds the service. A nil m turns the feature off. Who may review a
// post is ps's CheckEdit.
func New(store *repo.Store, ps *posts.Service, m proofread.Model, opts ...Option) *Service {
	s := &Service{
		store: store, posts: ps, model: m,
		limits: DefaultLimits, models: DefaultModels, log: zap.NewNop(), now: time.Now,
	}
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
