// Package config declares every setting of the pcom binary as a go-flags
// option with paired `long:` and `env:` tags, so `web --help` is the
// configuration reference. Commands embed the groups they need; packages
// receive plain values from the command, never read the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	mjconfig "github.com/can3p/gogo/sender/mailjet/config"
	"github.com/can3p/gogo/settings"
	"github.com/jessevdk/go-flags"
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

	SecureCookies   Switch `long:"secure-cookies" env:"SECURE_COOKIES" description:"Mark the session cookie Secure (turn off for plain-HTTP localhost)" optional:"true" optional-value:"true" default:"true"`
	HSTS            Switch `long:"hsts" env:"HSTS" description:"Send Strict-Transport-Security" optional:"true" optional-value:"true" default:"true"`
	StaticCache     Switch `long:"static-cache" env:"STATIC_CACHE" description:"Serve /static with a long immutable Cache-Control" optional:"true" optional-value:"true" default:"true"`
	MediaPermaCache Switch `long:"media-perma-cache" env:"MEDIA_PERMA_CACHE" description:"Keep resized user media in the media server's permanent cache" optional:"true" optional-value:"true" default:"true"`
	ShowErrors      Switch `long:"show-errors" env:"SHOW_ERRORS" description:"Show error details in pages (development only)" optional:"true" optional-value:"true" default:"false"`
	ReportPanics    Switch `long:"report-panics" env:"REPORT_PANICS" description:"Mail the admin address when a page panics" optional:"true" optional-value:"true" default:"true"`
	LogLevel        string `long:"log-level" env:"LOG_LEVEL" description:"Log level" choice:"debug" choice:"info" choice:"warn" choice:"error" default:"info"`
	EnablePprof     Switch `long:"enable-pprof" env:"ENABLE_PPROF" description:"Serve net/http/pprof on :8081" optional:"true" optional-value:"true" default:"false"`
}

// SlogLevel is LogLevel as a slog level.
func (w *Web) SlogLevel() slog.Level {
	var l slog.Level
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

// Mail is outgoing mail: the addresses, the queue, and the Mailjet API that
// delivers it (tommy in development and tests).
type Mail struct {
	SenderAddress string        `long:"sender-address" env:"SENDER_ADDRESS" description:"From address of every mail" required:"true"`
	AdminAddress  string        `long:"admin-address" env:"ADMIN_ADDRESS" description:"Where admin notifications (signups, page failures) go"`
	PollInterval  time.Duration `long:"email-poll-interval" env:"EMAIL_POLL_INTERVAL" description:"How often the mail queue is sent" default:"10s"`

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
}

// Serve is the settings of the serve command.
type Serve struct {
	Database Database `group:"Database"`
	Web      Web      `group:"Web"`
	Mail     Mail     `group:"Mail"`
	Limits   Limits   `group:"Limits"`
	Login    Login    `group:"Login"`
	Media    Media    `group:"User media" namespace:"user-media" env-namespace:"USER_MEDIA"`

	Translation Translation `group:"Translation" namespace:"translation" env-namespace:"TRANSLATION"`
}

// Validate checks what go-flags tags can't express: settings required only
// together with others. Run it after parsing.
func (s *Serve) Validate() error {
	return s.Translation.Validate()
}

// Translation is translating posts and RSS items for readers. Provider picks
// the backend; empty turns the feature off. Each backend has its own nested
// group, and only the selected backend's settings are required.
type Translation struct {
	Provider         string `long:"provider" env:"PROVIDER" description:"Translation backend; empty turns translation off" choice:"" choice:"azure"`
	UserDailyChars   int    `long:"user-daily-chars" env:"USER_DAILY_CHARS" description:"Characters one reader may have translated per day" default:"50000"`
	SiteMonthlyChars int    `long:"site-monthly-chars" env:"SITE_MONTHLY_CHARS" description:"Characters the whole site may have translated per month, background re-translations included" default:"2000000"`

	Azure AzureTranslator `group:"Azure Translator" namespace:"azure" env-namespace:"AZURE"`
}

// AzureTranslator is the Azure Translator backend (provider azure).
type AzureTranslator struct {
	Key      settings.Secret `long:"key" env:"KEY" description:"Azure Translator resource key; required with provider azure"`
	Region   string          `long:"region" env:"REGION" description:"Resource region, such as westeurope; required by regional and multi-service resources, empty for a global one"`
	Endpoint string          `long:"endpoint" env:"ENDPOINT" description:"Translator API endpoint" default:"https://api.cognitive.microsofttranslator.com"`
}

// Validate requires the selected backend's settings and ignores the others'.
func (t *Translation) Validate() error {
	if t.UserDailyChars < 0 || t.SiteMonthlyChars < 0 {
		return errors.New("translation character limits can't be negative")
	}

	switch t.Provider {
	case "azure":
		if t.Azure.Key.Reveal() == "" {
			return errors.New("translation provider azure needs $TRANSLATION_AZURE_KEY (--translation-azure-key)")
		}

		if t.Azure.Endpoint == "" {
			return errors.New("translation provider azure needs $TRANSLATION_AZURE_ENDPOINT (--translation-azure-endpoint)")
		}
	}

	return nil
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
