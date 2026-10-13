package claude_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/proofread/claude"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/stretchr/testify/require"
	wm "github.com/wiremock/go-wiremock"
)

const apiKey = "test-key"

var (
	grammarChoice = proofread.ModelChoice{Name: "claude-haiku-5-5", Effort: proofread.EffortLow}
	styleChoice   = proofread.ModelChoice{Name: "claude-sonnet-5-5", Effort: proofread.EffortMedium}
)

func TestMain(m *testing.M) {
	wiremock.Register("testdata/wiremock")

	code := m.Run()
	_ = wiremock.Cleanup()

	os.Exit(code)
}

// client talks to the shared WireMock; the test fails on a request no stub
// matches. Stubs match the request shape, so a call that answers proves it.
func client(t *testing.T, key string) (*claude.Client, *wiremock.WireMock) {
	t.Helper()

	if testing.Short() {
		t.Skip("starts a container")
	}

	w := wiremock.Shared(t)

	return claude.New(claude.Options{APIKey: key, Endpoint: w.Watch(t, "anthropic")}), w
}

// userHas matches a request whose user message contains text.
type userHas struct {
	wm.BasicParamMatcher
	text string
}

func (u userHas) ParseMatcher() map[string]any {
	return map[string]any{"matchesJsonPath": map[string]any{"expression": "$.messages[0].content[0].text", "contains": u.text}}
}

func (u userHas) MarshalJSON() ([]byte, error) { return json.Marshal(u.ParseMatcher()) }

// sent counts requests whose user message contains the marker and every
// further text.
func sent(marker string, more ...string) *wm.Request {
	r := wm.NewRequest("POST", wm.URLPathEqualTo("/anthropic/v1/messages")).
		WithBodyPattern(userHas{wm.MatchingJsonPath(""), marker})
	for _, m := range more {
		r = r.WithBodyPattern(userHas{wm.MatchingJsonPath(""), m})
	}

	return r
}

func TestGrammar_RequestShapeAndAnswer(t *testing.T) {
	c, w := client(t, apiKey)

	out, usage, err := c.Grammar(t.Context(), proofread.GrammarInput{Model: grammarChoice, Subject: "Title", Body: "I saw a cta. wm-grammar-ok"})
	require.NoError(t, err)
	require.Equal(t, []proofread.Suggestion{{
		Field: proofread.FieldBody, Kind: proofread.GrammarKindSpelling,
		Quote: "cta", Before: "I saw a ", Replacement: "cat", Explanation: "Typo.",
	}}, out.Suggestions)
	require.Equal(t, proofread.Usage{Model: grammarChoice.Name, InputTokens: 120, OutputTokens: 30}, usage)
	w.Verify(t, 1, sent("wm-grammar-ok", "Title"))
}

func TestStyle_AnswerAndMessageCarryPreviousReviewAndNote(t *testing.T) {
	c, w := client(t, apiKey)

	out, usage, err := c.Style(t.Context(), proofread.StyleInput{
		Model: styleChoice, Subject: "Title", Body: "Then it ended. wm-style-ok", AuthorNote: "my-author-note",
		Previous: &proofread.PreviousReview{
			Number: 2, Subject: "prev-subject", Body: "Older text", Summary: "prev-summary", AuthorNote: "prev-author-note",
			Notes: []proofread.PreviousNote{{
				ID: "n1", Field: proofread.FieldBody, Kind: proofread.NoteKindPacing, Quote: "Then it ended.",
				Text: "Abrupt.", Status: proofread.NoteStatusKept, KeepReason: "prev-reason",
			}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "A solid first draft.", out.Summary)
	require.Equal(t, []proofread.Note{{
		Field: proofread.FieldBody, Kind: proofread.NoteKindPacing, Quote: "Then it ended.",
		Text: "The ending is abrupt.", Continues: "n1",
	}}, out.Notes)
	require.Equal(t, proofread.Usage{Model: styleChoice.Name, InputTokens: 400, OutputTokens: 80}, usage)
	w.Verify(t, 1, sent("wm-style-ok", "Title", "my-author-note", "prev-subject", "prev-summary", "prev-author-note",
		`id="n1"`, `status="kept"`, "prev-reason"))
}

func TestCalls_RefusalAndMaxTokensAreErrors(t *testing.T) {
	c, w := client(t, apiKey)

	_, _, err := c.Grammar(t.Context(), proofread.GrammarInput{Model: grammarChoice, Body: "wm-grammar-refusal"})
	require.ErrorIs(t, err, claude.ErrRefused)
	w.Verify(t, 1, sent("wm-grammar-refusal"))

	_, _, err = c.Grammar(t.Context(), proofread.GrammarInput{Model: grammarChoice, Body: "wm-grammar-maxtokens"})
	require.ErrorIs(t, err, claude.ErrTruncated)
	w.Verify(t, 1, sent("wm-grammar-maxtokens"))
}

func TestCalls_UnauthorizedIsNotRetried(t *testing.T) {
	c, w := client(t, "wrong-key")

	_, _, err := c.Grammar(t.Context(), proofread.GrammarInput{Model: grammarChoice, Body: "wm-unauthorized"})

	var apiErr *anthropic.Error

	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 401, apiErr.StatusCode)
	w.Verify(t, 1, sent("wm-unauthorized"))
}

func TestCalls_RateLimitIsRetried(t *testing.T) {
	c, w := client(t, apiKey)

	out, _, err := c.Grammar(t.Context(), proofread.GrammarInput{Model: grammarChoice, Body: "wm-retry"})
	require.NoError(t, err)
	require.NotNil(t, out.Suggestions)
	require.Empty(t, out.Suggestions)
	w.Verify(t, 2, sent("wm-retry"))
}
