// Package claude is the proofread.Model over the Claude API, through the
// official Go SDK. It is its own package because services may not speak
// HTTP: the composition root builds it and hands it to the proofread service.
package claude

import (
	"context"
	"errors"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/can3p/pcom/pkg/proofread"
)

// Defaults of the settings, also the configuration's defaults.
const (
	DefaultEndpoint         = "https://api.anthropic.com"
	DefaultTimeout          = 90 * time.Second
	DefaultGrammarMaxTokens = 8_000
	DefaultStyleMaxTokens   = 16_000
)

// Options configure the client. A zero Endpoint, Timeout or max tokens keeps
// its default.
type Options struct {
	// APIKey is the Anthropic API key.
	APIKey string
	// Endpoint is the API's base URL (WireMock in tests).
	Endpoint string
	// Timeout bounds one request.
	Timeout time.Duration
	// GrammarMaxTokens and StyleMaxTokens are the most output tokens of a
	// grammar check and a style review.
	GrammarMaxTokens, StyleMaxTokens int
}

// Client calls the Claude API. Build it with New.
type Client struct {
	api  anthropic.Client
	opts Options
}

var _ proofread.Model = (*Client)(nil)

// errNotImplemented is returned until the calls are written.
var errNotImplemented = errors.New("claude: not implemented")

// New builds a client from explicit settings only: the SDK's environment
// defaults (ANTHROPIC_API_KEY, ANTHROPIC_BASE_URL and the like) are ignored.
// Retries are left at the SDK's default.
func New(opts Options) *Client {
	if opts.Endpoint == "" {
		opts.Endpoint = DefaultEndpoint
	}

	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}

	if opts.GrammarMaxTokens <= 0 {
		opts.GrammarMaxTokens = DefaultGrammarMaxTokens
	}

	if opts.StyleMaxTokens <= 0 {
		opts.StyleMaxTokens = DefaultStyleMaxTokens
	}

	api := anthropic.NewClient(
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(opts.APIKey),
		option.WithBaseURL(opts.Endpoint),
		option.WithRequestTimeout(opts.Timeout),
	)

	return &Client{api: api, opts: opts}
}

// Grammar asks the model for a grammar check.
func (c *Client) Grammar(context.Context, proofread.GrammarInput) (proofread.GrammarOutput, proofread.Usage, error) {
	return proofread.GrammarOutput{}, proofread.Usage{}, errNotImplemented
}

// Style asks the model for a style review.
func (c *Client) Style(context.Context, proofread.StyleInput) (proofread.StyleOutput, proofread.Usage, error) {
	return proofread.StyleOutput{}, proofread.Usage{}, errNotImplemented
}
