package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/stretchr/testify/require"
)

var _ = registerProofreadStubs()

// registerProofreadStubs hands the container the stubs of this package's
// proofreading tests; each matches a marker in the text it is sent.
func registerProofreadStubs() struct{} {
	wiremock.Register("testdata/wiremock/proofread")

	return struct{}{}
}

const (
	grammarBody = "I saw a cta wm-e2e-grammar"
	styleBody   = "wm-e2e-style. Then it ended."
)

// decode reads a JSON answer into a generic map.
func decode(t *testing.T, r *e2e.Response) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Body), &m))

	return m
}

// A grammar check answers the suggestions that apply and the counts left; a
// style review is stored as the post's latest, its notes can be settled, and
// Start fresh deletes it; somebody else's post is refused.
func TestProofread_GrammarAndStyle(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithWireMock())
	user, c := newLoggedIn(t, app)

	got := decode(t, c.PostJSON("/controls/action/proofread_grammar",
		map[string]any{"subject": "Cats", "body": grammarBody}).RequireStatus(http.StatusOK))
	sugg := got["suggestions"].([]any)
	require.Len(t, sugg, 1)
	require.Equal(t, map[string]any{
		"field": "body", "kind": "spelling", "quote": "cta", "before": "I saw a ", "occurrence": 0.0,
		"replacement": "cat", "explanation": "Typo.",
	}, sugg[0])
	require.Equal(t, map[string]any{"grammar": 9.0, "style": 3.0, "grammar_limit": 10.0, "style_limit": 3.0}, got["remaining"])

	post, err := factory.Post(context.Background(), app.DB, user.ID, factory.Published(), factory.WithBody(styleBody))
	require.NoError(t, err)

	stateURL := "/controls/proofread_style/" + post.ID
	require.Nil(t, decode(t, c.Get(stateURL).RequireStatus(http.StatusOK))["review"])

	got = decode(t, c.PostJSON("/controls/action/proofread_style",
		map[string]any{"post_id": post.ID, "subject": "Cats", "body": styleBody, "author_note": "be gentle"}).RequireStatus(http.StatusOK))
	require.Equal(t, 2.0, got["remaining"].(map[string]any)["style"])

	posted := got["review"].(map[string]any)
	require.Equal(t, "A solid first draft.", posted["summary"])

	pn := posted["notes"].([]any)
	require.Len(t, pn, 1)
	require.Equal(t, "The ending is abrupt.", pn[0].(map[string]any)["text"])
	require.Equal(t, "Then it ended.", pn[0].(map[string]any)["quote"])
	require.Equal(t, "pacing", pn[0].(map[string]any)["kind"])
	require.Equal(t, 0.0, pn[0].(map[string]any)["occurrence"])

	state := decode(t, c.Get(stateURL).RequireStatus(http.StatusOK))
	review := state["review"].(map[string]any)
	require.Equal(t, 1.0, review["number"])
	require.Equal(t, "be gentle", review["author_note"])

	notes := review["notes"].([]any)
	require.Len(t, notes, 1)
	noteID := notes[0].(map[string]any)["id"].(string)

	c.PostJSON("/controls/action/proofread_style_note",
		map[string]any{"note_id": noteID, "status": "kept", "reason": "on purpose"}).RequireStatus(http.StatusOK)

	note := decode(t, c.Get(stateURL))["review"].(map[string]any)["notes"].([]any)[0].(map[string]any)
	require.Equal(t, "kept", note["status"])
	require.Equal(t, "on purpose", note["keep_reason"])

	// somebody else's post is refused to read, reset and settle; nothing changes
	_, other := newLoggedIn(t, app)
	require.Equal(t, "Operation not allowed", decode(t, other.Get(stateURL).RequireStatus(http.StatusBadRequest))["explanation"])
	require.Equal(t, "Operation not allowed", decode(t, other.PostJSON("/controls/action/proofread_style_reset",
		map[string]any{"post_id": post.ID}).RequireStatus(http.StatusBadRequest))["explanation"])
	require.Equal(t, "Operation not allowed", decode(t, other.PostJSON("/controls/action/proofread_style_note",
		map[string]any{"note_id": noteID, "status": "done"}).RequireStatus(http.StatusBadRequest))["explanation"])

	state = decode(t, c.Get(stateURL).RequireStatus(http.StatusOK))
	require.NotNil(t, state["review"])
	require.Equal(t, "kept", state["review"].(map[string]any)["notes"].([]any)[0].(map[string]any)["status"])

	c.PostJSON("/controls/action/proofread_style_reset", map[string]any{"post_id": post.ID}).RequireStatus(http.StatusOK)
	require.Nil(t, decode(t, c.Get(stateURL).RequireStatus(http.StatusOK))["review"])
}

// Over the hourly limit a check is refused with a sentence.
func TestProofread_LimitMessage(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithWireMock(), e2e.WithEnv("PROOFREAD_GRAMMAR_PER_HOUR", "1"))
	_, c := newLoggedIn(t, app)
	in := map[string]any{"subject": "Cats", "body": grammarBody}

	c.PostJSON("/controls/action/proofread_grammar", in).RequireStatus(http.StatusOK)

	got := decode(t, c.PostJSON("/controls/action/proofread_grammar", in).RequireStatus(http.StatusBadRequest))
	require.Contains(t, got["explanation"], "You have used all 1 grammar checks for this hour. The next one is available in")
}

// With no key the feature is off and every route answers 404.
func TestProofread_OffAnswers404(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	_, c := newLoggedIn(t, app)

	for _, p := range []string{"proofread_grammar", "proofread_style", "proofread_style_note", "proofread_style_reset"} {
		c.PostJSON("/controls/action/"+p, map[string]any{}).RequireStatus(http.StatusNotFound)
	}

	c.Get("/controls/proofread_style/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
}
