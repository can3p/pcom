// Package config declares every setting of the pcom binary as a go-flags
// option with paired `long:` and `env:` tags, so `web --help` is the
// configuration reference. Commands embed the groups they need; packages
// receive plain values from the command, never read the environment.
package config

import (
	"fmt"
	"strconv"
	"time"

	mjconfig "github.com/can3p/gogo/sender/mailjet/config"
	"github.com/can3p/gogo/settings"
	"github.com/jessevdk/go-flags"
	"go.uber.org/zap/zapcore"
)

// NewParser returns a parser for the binary: options are spelled
// --user-media-endpoint and $USER_MEDIA_ENDPOINT. Run it with settings.Parse,
// which also rejects required settings that are set but empty.
func NewParser() *flags.Parser {
	p := flags.NewNamedParser("web", flags.HelpFlag|flags.PassDoubleDash)
	p.NamespaceDelimiter = "-"
	p.EnvNamespaceDelimiter = "_"

	return p
}

// Database is the connection every command that touches data needs.
type Database struct {
	URL string `long:"database-url" env:"DATABASE_URL" description:"Postgres connection URL" required:"true"`
}

// Web is the HTTP server's settings.
type Web struct {
	SiteRoot    string          `long:"site-root" env:"SITE_ROOT" description:"Public root URL of the site, such as https://pcom.example; absolute links start with it" required:"true"`
	SessionSalt settings.Secret `long:"session-salt" env:"SESSION_SALT" description:"Salt for session cookies and hashed values" required:"true"`
	Port        int             `long:"port" env:"PORT" description:"Port to listen on" default:"8080"`
	GinMode     string          `long:"gin-mode" env:"GIN_MODE" description:"gin mode: debug, release or test; gin's own default if empty"`
	HTMLDir     string          `long:"html-dir" env:"HTML_DIR" description:"Template directory, relative to the working directory" default:"client/html"`
	ForceSignup Switch          `long:"force-signup" env:"FORCE_SIGNUP" description:"Allow new signups even when system settings close them" optional:"true" optional-value:"true" default:"false"`
	StaticCDN   string          `long:"static-cdn" env:"STATIC_CDN" description:"Origin that serves /static, such as https://cdn.example; served by the app if empty"`

	ReadHeaderTimeout time.Duration `long:"read-header-timeout" env:"READ_HEADER_TIMEOUT" description:"How long a client may take to send the request headers" default:"10s"`
	ReadTimeout       time.Duration `long:"read-timeout" env:"READ_TIMEOUT" description:"How long a client may take to send a whole request, upload included" default:"60s"`
	WriteTimeout      time.Duration `long:"write-timeout" env:"WRITE_TIMEOUT" description:"How long a response may take from the end of the request headers; keep it above --proofread-timeout" default:"120s"`
	IdleTimeout       time.Duration `long:"idle-timeout" env:"IDLE_TIMEOUT" description:"How long an idle keep-alive connection stays open" default:"120s"`
	ShutdownTimeout   time.Duration `long:"shutdown-timeout" env:"SHUTDOWN_TIMEOUT" description:"How long requests in flight may run after SIGINT or SIGTERM before the server closes them" default:"4s"`

	SessionMaxAge          time.Duration `long:"session-max-age" env:"SESSION_MAX_AGE" description:"How long a session lasts after its last save; the session cookie and its row expire then" default:"720h"`
	SessionCleanupInterval time.Duration `long:"session-cleanup-interval" env:"SESSION_CLEANUP_INTERVAL" description:"How often expired sessions are deleted from the database" default:"1h"`

	SecureCookies   Switch `long:"secure-cookies" env:"SECURE_COOKIES" description:"Mark the session cookie Secure (turn off for plain-HTTP localhost)" optional:"true" optional-value:"true" default:"true"`
	HSTS            Switch `long:"hsts" env:"HSTS" description:"Send Strict-Transport-Security" optional:"true" optional-value:"true" default:"true"`
	StaticCache     Switch `long:"static-cache" env:"STATIC_CACHE" description:"Serve /static with a long immutable Cache-Control" optional:"true" optional-value:"true" default:"true"`
	MediaPermaCache Switch `long:"media-perma-cache" env:"MEDIA_PERMA_CACHE" description:"Keep resized user media in the media server's permanent cache" optional:"true" optional-value:"true" default:"true"`
	ShowErrors      Switch `long:"show-errors" env:"SHOW_ERRORS" description:"Show error details in pages (development only)" optional:"true" optional-value:"true" default:"false"`
	ReportPanics    Switch `long:"report-panics" env:"REPORT_PANICS" description:"Mail the admin address when a page panics" optional:"true" optional-value:"true" default:"true"`
	LogLevel        string `long:"log-level" env:"LOG_LEVEL" description:"Log level" choice:"debug" choice:"info" choice:"warn" choice:"error" default:"info"`
	LogFormat       string `long:"log-format" env:"LOG_FORMAT" description:"Log encoding on stdout: json for the deployed app, console for a terminal" choice:"json" choice:"console" default:"json"`
	EnablePprof     Switch `long:"enable-pprof" env:"ENABLE_PPROF" description:"Serve net/http/pprof on :8081" optional:"true" optional-value:"true" default:"false"`
	DevRoutes       Switch `long:"dev-routes" env:"DEV_ROUTES" description:"Serve the /dev pages (mail previews); local development only" optional:"true" optional-value:"true" default:"false"`
}

// ZapLevel is LogLevel as a zap level.
func (w *Web) ZapLevel() zapcore.Level {
	var l zapcore.Level
	_ = l.UnmarshalText([]byte(w.LogLevel)) // the choice tag admits only valid names

	return l
}

// Limits are the sizes users may not exceed.
type Limits struct {
	ProfileAboutMaxLength int `long:"profile-about-max-length" env:"PROFILE_ABOUT_MAX_LENGTH" description:"Most characters a user's About text may have" default:"6000"`
	CommentMaxLength      int `long:"comment-max-length" env:"COMMENT_MAX_LENGTH" description:"Most characters a comment may have" default:"6000"`
	PostBodyMaxLength     int `long:"post-body-max-length" env:"POST_BODY_MAX_LENGTH" description:"Most characters a post body may have" default:"20000"`
	PostSubjectMaxLength  int `long:"post-subject-max-length" env:"POST_SUBJECT_MAX_LENGTH" description:"Most characters a post subject may have" default:"100"`
	PromptMaxLength       int `long:"prompt-max-length" env:"PROMPT_MAX_LENGTH" description:"Most characters a prompt message may have" default:"1400"`
	UserStylesMaxLength   int `long:"user-styles-max-length" env:"USER_STYLES_MAX_LENGTH" description:"Most characters a user's custom CSS may have" default:"10000"`
	PageSize              int `long:"page-size" env:"PAGE_SIZE" description:"Items per page of the feed, explore, the index and a journal" default:"30"`
	RSSLimit              int `long:"rss-limit" env:"RSS_LIMIT" description:"Items in every RSS output" default:"50"`

	MaxBodySize   int64 `long:"max-body-size" env:"MAX_BODY_SIZE" description:"Most bytes of a request body; the upload routes add their own cap to it" default:"1048576"`
	MaxUploadSize int64 `long:"max-upload-size" env:"MAX_UPLOAD_SIZE" description:"Most bytes of an uploaded or imported image" default:"20971520"`
	MaxImportSize int64 `long:"max-import-size" env:"MAX_IMPORT_SIZE" description:"Most bytes of a blog archive to import" default:"104857600"`
}

// Feeds bounds fetching the RSS feeds users follow. The defaults are
// reader.Default* and feeds.DefaultPollInterval.
type Feeds struct {
	PollInterval time.Duration `long:"rss-poll-interval" env:"RSS_POLL_INTERVAL" description:"How often feeds due for a fetch are looked for and queued" default:"10s"`
	FetchTimeout time.Duration `long:"rss-fetch-timeout" env:"RSS_FETCH_TIMEOUT" description:"How long fetching one feed document may take" default:"5s"`
	ImageTimeout time.Duration `long:"rss-image-timeout" env:"RSS_IMAGE_TIMEOUT" description:"How long downloading one image of a feed item may take" default:"30s"`
	MaxFeedSize  int64         `long:"rss-max-feed-size" env:"RSS_MAX_FEED_SIZE" description:"Most bytes of a feed document" default:"5242880"`
}

// Email is how pcom checks the addresses users give. The defaults are
// emailaddr.DefaultMXTimeout, emailaddr.DefaultCacheTTL and
// emailaddr.DefaultCacheSize.
type Email struct {
	MXTimeout       time.Duration `long:"email-mx-timeout" env:"EMAIL_MX_TIMEOUT" description:"How long the DNS lookups of an address's domain may take; 0 turns the check off" default:"2s"`
	MXCache         time.Duration `long:"email-mx-cache" env:"EMAIL_MX_CACHE" description:"How long a domain the DNS check accepted is remembered (a refused one is checked again next time); 0 turns the cache off" default:"1h"`
	MXCacheSize     int           `long:"email-mx-cache-size" env:"EMAIL_MX_CACHE_SIZE" description:"How many domains the DNS check accepted are remembered; 0 turns the cache off" default:"10000"`
	AllowDisposable Switch        `long:"email-allow-disposable" env:"EMAIL_ALLOW_DISPOSABLE" description:"Accept throwaway addresses on the subscribe form; local development only" optional:"true" optional-value:"true" default:"false"`
}

// Login is the limits of login by emailed code. Their defaults are
// accounts.DefaultLoginLimits.
type Login struct {
	CodeLifetime        time.Duration `long:"login-code-lifetime" env:"LOGIN_CODE_LIFETIME" description:"How long a mailed login code works" default:"15m"`
	CodeTries           int           `long:"login-code-tries" env:"LOGIN_CODE_TRIES" description:"Wrong codes one login attempt allows" default:"5"`
	CodesMailed         int           `long:"login-codes-mailed" env:"LOGIN_CODES_MAILED" description:"Login codes mailed to one user per --login-codes-mailed-window" default:"3"`
	CodesMailedWindow   time.Duration `long:"login-codes-mailed-window" env:"LOGIN_CODES_MAILED_WINDOW" description:"Window of --login-codes-mailed" default:"15m"`
	WrongTries          int           `long:"login-wrong-tries" env:"LOGIN_WRONG_TRIES" description:"Wrong codes on one user's attempts per --login-wrong-tries-window before no code logs them in" default:"10"`
	WrongTriesWindow    time.Duration `long:"login-wrong-tries-window" env:"LOGIN_WRONG_TRIES_WINDOW" description:"Window of --login-wrong-tries" default:"1h"`
	UnconfirmedLifetime time.Duration `long:"login-unconfirmed-lifetime" env:"LOGIN_UNCONFIRMED_LIFETIME" description:"How long an account nobody confirmed with a code is kept" default:"24h"`
	PruneEvery          time.Duration `long:"login-prune-every" env:"LOGIN_PRUNE_EVERY" description:"How often old login attempts and unconfirmed accounts are deleted" default:"1h"`
}

// Jobs is the background job runner (pkg/jobs). Its defaults are
// jobs.Default*.
type Jobs struct {
	PollInterval time.Duration `long:"jobs-poll-interval" env:"JOBS_POLL_INTERVAL" description:"How often the job runner looks for jobs to work" default:"1s"`
	Workers      int           `long:"jobs-workers" env:"JOBS_WORKERS" description:"How many jobs the runner works at once" default:"10"`
	Retention    time.Duration `long:"jobs-retention" env:"JOBS_RETENTION" description:"How long finished jobs are kept" default:"24h"`
}

// Mail is outgoing mail: the addresses, the queue, and the Mailjet API that
// delivers it (tommy in development and tests).
type Mail struct {
	SenderAddress  string          `long:"sender-address" env:"SENDER_ADDRESS" description:"From address of every mail" required:"true"`
	AdminAddress   string          `long:"admin-address" env:"ADMIN_ADDRESS" description:"Where admin notifications (signups, page failures) go"`
	RetryIntervals []time.Duration `long:"email-retry-intervals" env:"EMAIL_RETRY_INTERVALS" env-delim:"," description:"Waits between attempts to send a mail; a mail fails after one attempt more than there are waits" default:"10s" default:"60s" default:"30m"`

	Mailjet mjconfig.Config `group:"Mailjet" namespace:"mj" env-namespace:"MJ"`
}

// Media is the S3-compatible bucket that stores user media. Embed it with
// `namespace:"user-media" env-namespace:"USER_MEDIA"`.
type Media struct {
	Endpoint  string          `long:"endpoint" env:"ENDPOINT" description:"S3 endpoint URL" required:"true"`
	Bucket    string          `long:"bucket" env:"BUCKET" description:"Bucket name" required:"true"`
	Region    string          `long:"region" env:"REGION" description:"Bucket region" required:"true"`
	Key       string          `long:"key" env:"KEY" description:"Access key ID" required:"true"`
	Secret    settings.Secret `long:"secret" env:"SECRET" description:"Secret access key" required:"true"`
	PathStyle Switch          `long:"path-style" env:"PATH_STYLE" description:"Path-style addressing (endpoint/bucket/key), as tommy needs" optional:"true" optional-value:"true" default:"false"`
	CDN       string          `long:"cdn" env:"CDN" description:"Origin that serves user media, such as https://media.example; served by the app if empty"`
	Timeout   time.Duration   `long:"timeout" env:"TIMEOUT" description:"How long one request to the bucket may take" default:"60s"`
}

// Proofread is the grammar check and style review of posts by Claude. Embed
// it with `namespace:"proofread" env-namespace:"PROOFREAD"`. Without a key
// the feature is off. The defaults are proofread.Default* and claude.Default*.
type Proofread struct {
	AnthropicKey      settings.Secret `long:"anthropic-key" env:"ANTHROPIC_KEY" description:"Anthropic API key; grammar checks and style reviews are off without one"`
	AnthropicEndpoint string          `long:"anthropic-endpoint" env:"ANTHROPIC_ENDPOINT" description:"Anthropic API base URL" default:"https://api.anthropic.com"`
	GrammarModel      string          `long:"grammar-model" env:"GRAMMAR_MODEL" description:"Model of grammar checks" default:"claude-haiku-5-5"`
	GrammarEffort     string          `long:"grammar-effort" env:"GRAMMAR_EFFORT" description:"Effort of grammar checks" choice:"low" choice:"medium" choice:"high" default:"low"`
	StyleModel        string          `long:"style-model" env:"STYLE_MODEL" description:"Model of style reviews" default:"claude-sonnet-5-5"`
	StyleEffort       string          `long:"style-effort" env:"STYLE_EFFORT" description:"Effort of style reviews" choice:"low" choice:"medium" choice:"high" default:"medium"`
	GrammarPerHour    int             `long:"grammar-per-hour" env:"GRAMMAR_PER_HOUR" description:"Grammar checks one user may make per hour" default:"10"`
	StylePerHour      int             `long:"style-per-hour" env:"STYLE_PER_HOUR" description:"Style reviews one user may make per hour" default:"3"`
	SiteDailyRequests int             `long:"site-daily-requests" env:"SITE_DAILY_REQUESTS" description:"Grammar checks and style reviews the whole site may make per day; 0 is no cap" default:"0"`
	NoteMaxLength     int             `long:"note-max-length" env:"NOTE_MAX_LENGTH" description:"Most characters of an author note to a style review, or of a reason to keep a note as is" default:"2000"`
	Timeout           time.Duration   `long:"timeout" env:"TIMEOUT" description:"How long one request to the Anthropic API may take" default:"90s"`
	GrammarMaxTokens  int             `long:"grammar-max-tokens" env:"GRAMMAR_MAX_TOKENS" description:"Most output tokens of a grammar check" default:"8000"`
	StyleMaxTokens    int             `long:"style-max-tokens" env:"STYLE_MAX_TOKENS" description:"Most output tokens of a style review" default:"16000"`
}

// Subscriptions is email subscriptions to authors and the checks of the
// public form that starts one. Embed it with `namespace:"subscribe"
// env-namespace:"SUBSCRIBE"`. The defaults are subscriptions.Default* and
// abuse.Default*.
type Subscriptions struct {
	ConfirmLifetime       time.Duration `long:"confirm-lifetime" env:"CONFIRM_LIFETIME" description:"How long a mailed subscription confirmation link works" default:"72h"`
	ConfirmsPerAddress    int           `long:"confirms-per-address" env:"CONFIRMS_PER_ADDRESS" description:"Subscription confirmation mails one address may get" default:"3"`
	ManageLinksPerAddress int           `long:"manage-links-per-address" env:"MANAGE_LINKS_PER_ADDRESS" description:"Mails with the link to manage subscriptions one address may get" default:"3"`
	ConfirmsPerHour       int           `long:"confirms-per-hour" env:"CONFIRMS_PER_HOUR" description:"Subscription confirmation mails the whole site may send per hour" default:"100"`
	DomainLookupsPerHour  int           `long:"domain-lookups-per-hour" env:"DOMAIN_LOOKUPS_PER_HOUR" description:"DNS lookups of subscribing addresses' domains the whole site may make per hour; past it addresses go unchecked" default:"300"`
	PowDifficulty         int           `long:"pow-difficulty" env:"POW_DIFFICULTY" description:"Leading zero bits of the proof of work the subscribe form's browser solves" default:"15"`
	PowMaxAge             time.Duration `long:"pow-max-age" env:"POW_MAX_AGE" description:"How long a proof-of-work challenge may be answered" default:"30m"`
	MinFillTime           time.Duration `long:"min-fill-time" env:"MIN_FILL_TIME" description:"Least time between showing the subscribe form and submitting it" default:"2s"`
	PruneEvery            time.Duration `long:"prune-every" env:"PRUNE_EVERY" description:"How often expired subscriptions and old subscription mail records are deleted" default:"1h"`
}

// Notes is the limits of notes and their captures. The defaults are
// notes.Default*.
type Notes struct {
	BodyMaxLength        int           `long:"note-body-max-length" env:"NOTE_BODY_MAX_LENGTH" description:"Most characters a note may have" default:"20000"`
	TagMaxLength         int           `long:"note-tag-max-length" env:"NOTE_TAG_MAX_LENGTH" description:"Most characters of a note tag" default:"40"`
	TagsMax              int           `long:"note-tags-max" env:"NOTE_TAGS_MAX" description:"Most tags set by hand on one note" default:"10"`
	SourceTitleMaxLength int           `long:"note-source-title-max-length" env:"NOTE_SOURCE_TITLE_MAX_LENGTH" description:"Most characters of the title of a note's source" default:"300"`
	PhotoMaxDimension    int           `long:"note-photo-max-dimension" env:"NOTE_PHOTO_MAX_DIMENSION" description:"Longest side in pixels a note's photo is scaled down to" default:"2048"`
	AudioMaxDuration     time.Duration `long:"note-audio-max-duration" env:"NOTE_AUDIO_MAX_DURATION" description:"Longest recording of one note" default:"10m"`
	AudioMaxBytes        int64         `long:"note-audio-max-bytes" env:"NOTE_AUDIO_MAX_BYTES" description:"Most bytes of one note's recording" default:"8388608"`
	AudioBitrate         int           `long:"note-audio-bitrate" env:"NOTE_AUDIO_BITRATE" description:"Bits per second the browser records notes at" default:"24000"`
	CaptureMaxBytes      int64         `long:"note-capture-max-bytes" env:"NOTE_CAPTURE_MAX_BYTES" description:"Most bytes of one capture with its photos and recording" default:"33554432"`
	QuoteSourceWindow    time.Duration `long:"note-quote-source-window" env:"NOTE_QUOTE_SOURCE_WINDOW" description:"How long after a note with a source a new quote still takes that source" default:"12h"`
	PanelTags            int           `long:"note-panel-tags" env:"NOTE_PANEL_TAGS" description:"How many of the author's most used tags the post editor's Notes panel offers" default:"8"`
}

// STTProviderOpenAICompat is the OpenAI-compatible speech-to-text backend.
const STTProviderOpenAICompat = "openai-compat"

// STT is the speech-to-text backend that transcribes notes' recordings.
// Embed it with `namespace:"stt" env-namespace:"STT"`. Without a provider
// transcription is off. The defaults are notes.Default*.
type STT struct {
	Provider          string        `long:"provider" env:"PROVIDER" description:"Speech-to-text backend; empty turns transcription off" choice:"" choice:"openai-compat" default:""`
	Timeout           time.Duration `long:"timeout" env:"TIMEOUT" description:"How long one call to the backend may take" default:"2m"`
	Workers           int           `long:"workers" env:"WORKERS" description:"How many transcriptions run at once" default:"2"`
	Attempts          int           `long:"attempts" env:"ATTEMPTS" description:"How many times a transcription is tried before the note is marked failed" default:"5"`
	MinutesPerDay     int           `long:"minutes-per-day" env:"MINUTES_PER_DAY" description:"Minutes of recordings one user may have transcribed per day" default:"60"`
	SiteMinutesPerDay int           `long:"site-minutes-per-day" env:"SITE_MINUTES_PER_DAY" description:"Minutes of recordings the whole site may have transcribed per day; 0 is no cap" default:"0"`

	OpenAICompat STTOpenAICompat `group:"OpenAI-compatible speech-to-text" namespace:"openai-compat" env-namespace:"OPENAI_COMPAT"`
}

// STTOpenAICompat is a backend speaking OpenAI's /audio/transcriptions API.
type STTOpenAICompat struct {
	Endpoint string          `long:"endpoint" env:"ENDPOINT" description:"Base URL of the API" default:"https://api.groq.com/openai/v1"`
	Model    string          `long:"model" env:"MODEL" description:"Transcription model" default:"whisper-large-v3-turbo"`
	Key      settings.Secret `long:"key" env:"KEY" description:"API key; required with --stt-provider=openai-compat"`
}

// Validate rejects a selected backend without its key.
func (s STT) Validate() error {
	if s.Provider == STTProviderOpenAICompat && s.OpenAICompat.Key.Reveal() == "" {
		return fmt.Errorf("--stt-provider=%s needs --stt-openai-compat-key ($STT_OPENAI_COMPAT_KEY)", STTProviderOpenAICompat)
	}

	return nil
}

// Serve is the settings of the serve command.
type Serve struct {
	Database  Database  `group:"Database"`
	Web       Web       `group:"Web"`
	Mail      Mail      `group:"Mail"`
	Jobs      Jobs      `group:"Jobs"`
	Limits    Limits    `group:"Limits"`
	Login     Login     `group:"Login"`
	Email     Email     `group:"Email"`
	Feeds     Feeds     `group:"RSS feeds"`
	Media     Media     `group:"User media" namespace:"user-media" env-namespace:"USER_MEDIA"`
	Proofread Proofread `group:"Proofread" namespace:"proofread" env-namespace:"PROOFREAD"`

	Subscriptions Subscriptions `group:"Email subscriptions" namespace:"subscribe" env-namespace:"SUBSCRIBE"`

	Notes Notes `group:"Notes"`
	STT   STT   `group:"Speech to text" namespace:"stt" env-namespace:"STT"`
}

// Validate checks what go-flags can't: settings that depend on each other.
// The serve command calls it before it starts.
func (s Serve) Validate() error {
	return s.STT.Validate()
}

// Switch is a boolean setting that may default to true, which go-flags
// doesn't allow for a bool. `--name` alone and NAME=true turn it on,
// `--name=false` and NAME=false off; an empty or unparsable value is an error
// rather than a silent default.
type Switch struct{ on bool }

// On reports whether the switch is on.
func (s Switch) On() bool { return s.on }

// UnmarshalFlag parses a value strconv.ParseBool accepts.
func (s *Switch) UnmarshalFlag(v string) error {
	on, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("want true or false, got %q", v)
	}

	s.on = on

	return nil
}

// MarshalFlag prints the switch for --help.
func (s Switch) MarshalFlag() (string, error) { return strconv.FormatBool(s.on), nil }
