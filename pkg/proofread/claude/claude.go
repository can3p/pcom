// Package claude is the proofread.Model over the Claude API, through the
// official Go SDK. It is its own package because services may not speak
// HTTP: the composition root builds it and hands it to the proofread service.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// Errors a call returns, wrapped with detail. Any other error is the SDK's
// (an *anthropic.Error for an HTTP status, a transport error otherwise).
var (
	// ErrRefused is a call the model refused to answer.
	ErrRefused = errors.New("claude: the model refused")
	// ErrTruncated is an answer cut off by the max_tokens limit.
	ErrTruncated = errors.New("claude: the answer hit max_tokens")
	// ErrBadAnswer is an answer that is not the JSON the schema asks for.
	ErrBadAnswer = errors.New("claude: unusable answer")
)

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
func (c *Client) Grammar(ctx context.Context, in proofread.GrammarInput) (proofread.GrammarOutput, proofread.Usage, error) {
	var out proofread.GrammarOutput

	user := "<subject>\n" + in.Subject + "\n</subject>\n\n<body>\n" + in.Body + "\n</body>"

	usage, err := c.call(ctx, in.Model, proofread.GrammarSystemPrompt(), user,
		proofread.GrammarSchema(), c.opts.GrammarMaxTokens, &out)
	if err != nil {
		return proofread.GrammarOutput{}, usage, err
	}

	if out.Suggestions == nil {
		out.Suggestions = []proofread.Suggestion{}
	}

	return out, usage, nil
}

// Style asks the model for a style review.
func (c *Client) Style(ctx context.Context, in proofread.StyleInput) (proofread.StyleOutput, proofread.Usage, error) {
	var out proofread.StyleOutput

	usage, err := c.call(ctx, in.Model, proofread.StyleSystemPrompt(), styleMessage(in),
		proofread.StyleSchema(), c.opts.StyleMaxTokens, &out)
	if err != nil {
		return proofread.StyleOutput{}, usage, err
	}

	if out.Notes == nil {
		out.Notes = []proofread.Note{}
	}

	return out, usage, nil
}

// styleMessage is the compact state of a review: the text, the previous
// review if any, and the author's note.
func styleMessage(in proofread.StyleInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<current>\n<subject>\n%s\n</subject>\n\n<body>\n%s\n</body>\n</current>\n", in.Subject, in.Body)

	if p := in.Previous; p != nil {
		fmt.Fprintf(&b, "\n<previous_review number=%q>\n<subject>\n%s\n</subject>\n\n<body>\n%s\n</body>\n\n<summary>\n%s\n</summary>\n",
			strconv.Itoa(p.Number), p.Subject, p.Body, p.Summary)

		if p.AuthorNote != "" {
			fmt.Fprintf(&b, "\n<author_note>\n%s\n</author_note>\n", p.AuthorNote)
		}

		for _, n := range p.Notes {
			fmt.Fprintf(&b, "\n<note id=%q field=%q kind=%q status=%q>\n", n.ID, n.Field, n.Kind, n.Status)

			if n.Quote != "" {
				fmt.Fprintf(&b, "<quote>%s</quote>\n", n.Quote)
			}

			fmt.Fprintf(&b, "<text>%s</text>\n", n.Text)

			if n.KeepReason != "" {
				fmt.Fprintf(&b, "<keep_reason>%s</keep_reason>\n", n.KeepReason)
			}

			b.WriteString("</note>\n")
		}

		b.WriteString("</previous_review>\n")
	}

	if in.AuthorNote != "" {
		fmt.Fprintf(&b, "\n<note_for_this_review>\n%s\n</note_for_this_review>\n", in.AuthorNote)
	}

	return b.String()
}

// call sends one structured-output request and decodes the answer into out.
// The system prompt is a cached block. A refusal or max_tokens is an error.
func (c *Client) call(ctx context.Context, choice proofread.ModelChoice, system, user string,
	schema map[string]any, maxTokens int, out any,
) (proofread.Usage, error) {
	usage := proofread.Usage{Model: choice.Name}

	msg, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     choice.Name,
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(choice.Effort),
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	})
	if err != nil {
		return usage, fmt.Errorf("claude: %s: %w", choice.Name, err)
	}

	if msg.Model != "" {
		usage.Model = string(msg.Model)
	}

	usage.InputTokens = int(msg.Usage.InputTokens)
	usage.OutputTokens = int(msg.Usage.OutputTokens)

	switch msg.StopReason {
	case anthropic.StopReasonRefusal:
		return usage, ErrRefused
	case anthropic.StopReasonMaxTokens:
		return usage, ErrTruncated
	}

	var text strings.Builder

	for _, block := range msg.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return usage, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}

	return usage, nil
}
