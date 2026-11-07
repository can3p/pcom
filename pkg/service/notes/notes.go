// Package notes is a user's private notes: text, photos and recordings
// captured on the go, their tags, sources and focus, and the transcription of
// recordings by a speech-to-text backend. Every note is its owner's alone:
// anybody else is told it doesn't exist.
package notes

import (
	"context"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
)

// Defaults of the limits, also the configuration's defaults.
const (
	DefaultBodyMaxLength        = 20000
	DefaultTagMaxLength         = 40
	DefaultTagsMax              = 10
	DefaultSourceTitleMaxLength = 300
	DefaultPhotoMaxDimension    = 2048
	DefaultAudioMaxDuration     = 10 * time.Minute
	DefaultAudioMaxBytes        = 8 << 20
	DefaultAudioBitrate         = 24000
	DefaultCaptureMaxBytes      = 32 << 20
	DefaultSessionGap           = 30 * time.Minute
	DefaultFocusIdle            = 12 * time.Hour
	// DefaultPageSize is how many notes a page of the list holds unless
	// WithPageSize sets it.
	DefaultPageSize = 30

	DefaultSTTMinutesPerDay     = 60
	DefaultSTTSiteMinutesPerDay = 0 // no cap
)

// Defaults of the speech-to-text settings the runner and the backend take,
// also the configuration's defaults.
const (
	// DefaultSTTTimeout is how long one call to the backend may take.
	DefaultSTTTimeout = 2 * time.Minute
	// DefaultSTTWorkers is how many transcriptions run at once, on QueueSTT.
	DefaultSTTWorkers = 2
	// DefaultSTTEndpoint and DefaultSTTModel are the OpenAI-compatible
	// backend's base URL and model.
	DefaultSTTEndpoint = "https://api.groq.com/openai/v1"
	DefaultSTTModel    = "whisper-large-v3-turbo"
)

// QueueSTT is the River queue transcriptions run on, so a slow backend never
// holds up mail on the default queue.
const QueueSTT = "stt"

// Limits are the sizes notes may not exceed and the times that group them.
type Limits struct {
	BodyMaxLength        int
	TagMaxLength         int
	TagsMax              int
	SourceTitleMaxLength int
	// PhotoMaxDimension is the longest side a photo is scaled down to.
	PhotoMaxDimension int
	AudioMaxDuration  time.Duration
	AudioMaxBytes     int64
	// AudioBitrate is the bits per second the browser records at.
	AudioBitrate int
	// CaptureMaxBytes is the most bytes of one capture with all its files.
	CaptureMaxBytes int64
	// SessionGap is the most time between two captures of one session.
	SessionGap time.Duration
	// FocusIdle is how long the focus lasts after the last capture under it.
	FocusIdle time.Duration
	// STTMinutesPerDay is the minutes of recordings one user may have
	// transcribed per day; STTSiteMinutesPerDay those of the whole site,
	// zero meaning no cap.
	STTMinutesPerDay, STTSiteMinutesPerDay int
}

// DefaultLimits are the limits of a service built without configuration.
var DefaultLimits = Limits{
	BodyMaxLength:        DefaultBodyMaxLength,
	TagMaxLength:         DefaultTagMaxLength,
	TagsMax:              DefaultTagsMax,
	SourceTitleMaxLength: DefaultSourceTitleMaxLength,
	PhotoMaxDimension:    DefaultPhotoMaxDimension,
	AudioMaxDuration:     DefaultAudioMaxDuration,
	AudioMaxBytes:        DefaultAudioMaxBytes,
	AudioBitrate:         DefaultAudioBitrate,
	CaptureMaxBytes:      DefaultCaptureMaxBytes,
	SessionGap:           DefaultSessionGap,
	FocusIdle:            DefaultFocusIdle,
	STTMinutesPerDay:     DefaultSTTMinutesPerDay,
	STTSiteMinutesPerDay: DefaultSTTSiteMinutesPerDay,
}

// Service is the notes of every user.
type Service struct {
	store  *repo.Store
	jobs   repo.JobQueue
	media  server.MediaStorage
	stt    STT
	limits Limits
	// pageSize is how many notes a page of List holds.
	pageSize int
	now      func() time.Time
}

// Option changes how New builds the service.
type Option func(*Service)

// WithJobs sets the queue the service enqueues its background jobs on.
func WithJobs(q repo.JobQueue) Option {
	return func(s *Service) { s.jobs = q }
}

// WithMediaStorage sets the bucket photos and recordings are stored in.
func WithMediaStorage(m server.MediaStorage) Option {
	return func(s *Service) { s.media = m }
}

// WithSTT sets the speech-to-text backend; nil keeps transcription off.
func WithSTT(stt STT) Option {
	return func(s *Service) { s.stt = stt }
}

// WithPageSize sets how many notes a page of the list holds; a value below
// one keeps the default.
func WithPageSize(n int) Option {
	return func(s *Service) { setIfPositive(&s.pageSize, n) }
}

// WithClock replaces the clock captures and the focus are timed by, for
// tests.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLimits sets the limits; a zero field keeps its default (for
// STTSiteMinutesPerDay the default is no cap).
func WithLimits(l Limits) Option {
	return func(s *Service) {
		setIfPositive(&s.limits.BodyMaxLength, l.BodyMaxLength)
		setIfPositive(&s.limits.TagMaxLength, l.TagMaxLength)
		setIfPositive(&s.limits.TagsMax, l.TagsMax)
		setIfPositive(&s.limits.SourceTitleMaxLength, l.SourceTitleMaxLength)
		setIfPositive(&s.limits.PhotoMaxDimension, l.PhotoMaxDimension)
		setIfPositive(&s.limits.AudioMaxDuration, l.AudioMaxDuration)
		setIfPositive(&s.limits.AudioMaxBytes, l.AudioMaxBytes)
		setIfPositive(&s.limits.AudioBitrate, l.AudioBitrate)
		setIfPositive(&s.limits.CaptureMaxBytes, l.CaptureMaxBytes)
		setIfPositive(&s.limits.SessionGap, l.SessionGap)
		setIfPositive(&s.limits.FocusIdle, l.FocusIdle)
		setIfPositive(&s.limits.STTMinutesPerDay, l.STTMinutesPerDay)
		setIfPositive(&s.limits.STTSiteMinutesPerDay, l.STTSiteMinutesPerDay)
	}
}

func setIfPositive[T int | int64 | time.Duration](field *T, v T) {
	if v > 0 {
		*field = v
	}
}

// New builds the service. It takes no other service.
func New(store *repo.Store, opts ...Option) *Service {
	s := &Service{
		store:    store,
		limits:   DefaultLimits,
		pageSize: DefaultPageSize,
		now:      time.Now,
	}

	for _, o := range opts {
		o(s)
	}

	return s
}

// Limits are the service's limits.
func (s *Service) Limits() Limits { return s.limits }

// TranscriptionEnabled reports whether recordings are transcribed.
func (s *Service) TranscriptionEnabled() bool { return s.stt != nil }

// RegisterJobs registers the workers of the service's job kinds on r.
func (s *Service) RegisterJobs(_ *jobs.Registry) {}

// access is what a method does with a note.
type access int

const (
	// read loads the note.
	read access = iota
	// write locks the note for update until store's transaction ends.
	write
)

// ownNote is the authorization every method that names a note goes through:
// an anonymous actor needs to log in, and a note that isn't the actor's
// doesn't exist for them. With write, store must be a transaction.
func (s *Service) ownNote(ctx context.Context, store *repo.Store, actor *model.User, noteID string, a access) (*model.Note, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	load := store.NoteByID
	if a == write {
		load = store.LockNote
	}

	note, err := load(ctx, noteID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if note.UserID != actor.ID {
		return nil, service.ErrNotFound
	}

	return note, nil
}
