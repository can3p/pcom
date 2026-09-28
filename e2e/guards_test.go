package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

const e2Password = "guard-secret-pw"

// e2World is the fixture every guard test acts on: an owner who is logged in,
// plus the objects the mutating routes could touch.
type e2World struct {
	app *e2e.App

	owner, friend, stranger, requester *core.User

	draft, published   *core.Post
	comment            *core.PostComment
	request            *core.UserConnectionMediationRequest
	invitation         *core.UserInvitation
	subscription       *core.UserFeedSubscription
	feedItem           *core.UserFeedItem
	prompt             *core.PostPrompt
	ownerClient        *e2e.Client
	ownerSessionCookie []*http.Cookie
}

func e2NewWorld(t *testing.T, app *e2e.App) *e2World {
	t.Helper()

	ctx := context.Background()
	db := app.DB
	w := &e2World{app: app}

	var err error

	w.owner = e2User(t, app)
	w.friend = e2User(t, app)
	w.stranger = e2User(t, app)
	w.requester = e2User(t, app)

	_, _, err = factory.Connect(ctx, db, w.owner.ID, w.friend.ID)
	require.NoError(t, err)

	w.draft, err = factory.Post(ctx, db, w.owner.ID)
	require.NoError(t, err)
	w.published, err = factory.Post(ctx, db, w.owner.ID, factory.Published())
	require.NoError(t, err)
	w.comment, err = factory.Comment(ctx, db, w.published.ID, w.friend.ID)
	require.NoError(t, err)

	w.request, err = factory.MediationRequest(ctx, db, w.requester.ID, w.owner.ID)
	require.NoError(t, err)
	w.invitation, err = factory.Invitation(ctx, db, w.owner.ID)
	require.NoError(t, err)

	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)
	w.subscription, err = factory.Subscription(ctx, db, w.owner.ID, feed.ID)
	require.NoError(t, err)
	item, err := factory.RSSItem(ctx, db, feed.ID)
	require.NoError(t, err)
	w.feedItem, err = factory.UserFeedItem(ctx, db, w.owner.ID, item.ID)
	require.NoError(t, err)

	w.prompt, err = factory.PostPrompt(ctx, db, w.friend.ID, w.owner.ID)
	require.NoError(t, err)

	require.NoError(t, factory.SetRegistrationOpen(ctx, db, true))

	w.ownerClient = app.Client(t)
	w.ownerClient.LoginAs(w.owner.Email, e2Password)
	w.ownerSessionCookie = w.ownerClient.Cookies()

	return w
}

func e2User(t *testing.T, app *e2e.App) *core.User {
	t.Helper()

	u, err := factory.User(context.Background(), app.DB, factory.WithPassword(e2Password))
	require.NoError(t, err)

	return u
}

// e2Snapshot is the part of the world a mutating route could change.
type e2Snapshot struct {
	Users      []string
	Posts      []string
	Comments   []string
	Emails     int
	Connected  bool
	Request    string
	Mediators  int
	Subscribed bool
	ItemHidden bool
	Prompt     bool
	LoggedInAs string
}

func (w *e2World) snapshot(t *testing.T) e2Snapshot {
	t.Helper()

	ctx := context.Background()
	db := w.app.DB

	var s e2Snapshot

	for _, u := range []*core.User{w.owner, w.friend, w.stranger, w.requester} {
		got, err := factory.GetUser(ctx, db, u.ID)
		require.NoError(t, err)
		s.Users = append(s.Users, fmt.Sprintf("%s|%s|%s|%s|%s|%v",
			got.Email, got.Username, got.Pwdhash.String, got.Timezone, got.ProfileVisibility, got.UpdatedAt.Time))

		posts, err := factory.ListPosts(ctx, db, u.ID)
		require.NoError(t, err)

		for _, p := range posts {
			s.Posts = append(s.Posts, fmt.Sprintf("%s|%s|%s|%s|%v", p.ID, p.Subject.String, p.Body, p.VisibilityRadius, p.PublishedAt.Valid))

			comments, err := factory.ListComments(ctx, db, p.ID)
			require.NoError(t, err)

			for _, c := range comments {
				s.Comments = append(s.Comments, c.ID+"|"+c.Body)
			}
		}
	}

	slices.Sort(s.Posts)
	slices.Sort(s.Comments)

	emails, err := factory.ListOutgoingEmails(ctx, db)
	require.NoError(t, err)
	s.Emails = len(emails)

	s.Connected, err = factory.ConnectionExists(ctx, db, w.owner.ID, w.friend.ID)
	require.NoError(t, err)

	req, err := factory.GetMediationRequest(ctx, db, w.request.ID)
	require.NoError(t, err)
	s.Request = fmt.Sprintf("%v|%v", req.TargetDecision, req.ConnectionID)

	decisions, err := factory.ListMediatorDecisions(ctx, db, w.request.ID)
	require.NoError(t, err)
	s.Mediators = len(decisions)

	s.Subscribed, err = factory.SubscriptionExists(ctx, db, w.owner.ID, w.subscription.FeedID)
	require.NoError(t, err)

	item, err := factory.GetUserFeedItem(ctx, db, w.feedItem.ID)
	require.NoError(t, err)
	s.ItemHidden = item.IsDismissed

	prompt, err := factory.GetPostPrompt(ctx, db, w.prompt.ID)
	require.NoError(t, err)
	s.Prompt = prompt.DismissedAt.Valid

	s.LoggedInAs = e2CurrentUsername(t, w.ownerClient)

	return s
}

// e2CurrentUsername is the username the navbar greets on /feed, or "" when the
// session is anonymous.
func e2CurrentUsername(t *testing.T, c *e2e.Client) string {
	t.Helper()

	resp := c.Get("/feed")
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	return strings.TrimSpace(resp.Doc().Find(".navbar-text a").First().Text())
}

// e2Route is one mutating or EnforceAuth route, as registered in cmd/web.
type e2Route struct {
	method  string
	pattern string
	// path returns the concrete path for the pattern.
	path func(w *e2World) string
	// payload is what a legitimate request would send, so that a guard that
	// let the request through would visibly change the database.
	payload func(w *e2World) map[string]string
}

func (r e2Route) key() string { return r.method + " " + r.pattern }

func (r e2Route) isJSON() bool {
	return strings.HasPrefix(r.pattern, "/controls/action/")
}

func e2Static(p string) func(*e2World) string { return func(*e2World) string { return p } }

func e2None(*e2World) map[string]string { return nil }

// e2Routes is every POST/PUT/DELETE and EnforceAuth route of cmd/web outside
// /api/v1. TestE2RouteTableMatchesSource keeps it in sync with the source.
var e2Routes = []e2Route{
	// EnforceAuth GET routes
	{http.MethodGet, "/posts/:id/edit", func(w *e2World) string { return "/posts/" + w.draft.ID + "/edit" }, e2None},
	{http.MethodGet, "/write", e2Static("/write"), e2None},
	{http.MethodGet, "/feed", e2Static("/feed"), e2None},
	{http.MethodGet, "/controls/", e2Static("/controls/"), e2None},
	{http.MethodGet, "/controls/settings", e2Static("/controls/settings"), e2None},

	// /controls/action (actions.go and logout)
	{http.MethodPost, "/controls/action/logout", e2Static("/controls/action/logout"), e2None},
	{http.MethodPost, "/controls/action/remove_from_whitelist", e2Static("/controls/action/remove_from_whitelist"),
		func(w *e2World) map[string]string { return map[string]string{"userId": w.stranger.ID} }},
	{http.MethodPost, "/controls/action/create_connection", e2Static("/controls/action/create_connection"),
		func(w *e2World) map[string]string { return map[string]string{"userId": w.stranger.ID} }},
	{http.MethodPost, "/controls/action/drop_connection", e2Static("/controls/action/drop_connection"),
		func(w *e2World) map[string]string { return map[string]string{"userId": w.friend.ID} }},
	{http.MethodPost, "/controls/action/request_mediation", e2Static("/controls/action/request_mediation"),
		func(w *e2World) map[string]string {
			return map[string]string{"userId": w.stranger.ID, "mediation_note": "hi"}
		}},
	{http.MethodPost, "/controls/action/revoke_mediation_request", e2Static("/controls/action/revoke_mediation_request"),
		func(w *e2World) map[string]string { return map[string]string{"userId": w.stranger.ID} }},
	{http.MethodPost, "/controls/action/dismiss_mediation", e2Static("/controls/action/dismiss_mediation"),
		func(w *e2World) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/sign_mediation", e2Static("/controls/action/sign_mediation"),
		func(w *e2World) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/reject_connection", e2Static("/controls/action/reject_connection"),
		func(w *e2World) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/accept_connection", e2Static("/controls/action/accept_connection"),
		func(w *e2World) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/delete_draft", e2Static("/controls/action/delete_draft"),
		func(w *e2World) map[string]string { return map[string]string{"postId": w.draft.ID} }},
	{http.MethodPost, "/controls/action/generate_api_key", e2Static("/controls/action/generate_api_key"), e2None},
	{http.MethodPost, "/controls/action/dismiss_prompt", e2Static("/controls/action/dismiss_prompt"),
		func(w *e2World) map[string]string { return map[string]string{"promptId": w.prompt.ID} }},
	{http.MethodPost, "/controls/action/remove_rss_subscription", e2Static("/controls/action/remove_rss_subscription"),
		func(w *e2World) map[string]string { return map[string]string{"id": w.subscription.ID} }},
	{http.MethodPost, "/controls/action/dissmiss_rss_item", e2Static("/controls/action/dissmiss_rss_item"),
		func(w *e2World) map[string]string { return map[string]string{"id": w.feedItem.ID} }},
	{http.MethodPost, "/controls/action/create_share", e2Static("/controls/action/create_share"),
		func(w *e2World) map[string]string { return map[string]string{"postId": w.published.ID} }},
	{http.MethodPost, "/controls/action/delete_share", e2Static("/controls/action/delete_share"),
		func(w *e2World) map[string]string { return map[string]string{"postId": w.published.ID} }},
	{http.MethodPost, "/controls/action/upload_media", e2Static("/controls/action/upload_media"), e2None},
	{http.MethodPost, "/controls/action/settings/export", e2Static("/controls/action/settings/export"), e2None},
	{http.MethodPost, "/controls/action/settings/import", e2Static("/controls/action/settings/import"), e2None},

	// /controls/form
	{http.MethodPost, "/controls/form/whitelist_connection", e2Static("/controls/form/whitelist_connection"),
		func(w *e2World) map[string]string { return map[string]string{"uname": w.stranger.Username} }},
	{http.MethodPost, "/controls/form/send_invite", e2Static("/controls/form/send_invite"),
		func(*e2World) map[string]string { return map[string]string{"email": "invitee@example.test"} }},
	{http.MethodPost, "/controls/form/edit_post", e2Static("/controls/form/edit_post"),
		func(w *e2World) map[string]string {
			return map[string]string{
				"post_id": w.published.ID, "subject": "hijacked", "body": "hijacked body",
				"visibility": string(core.PostVisibilityDirectOnly), "save_action": "save_post",
			}
		}},
	{http.MethodPost, "/controls/form/new_comment", e2Static("/controls/form/new_comment"),
		func(w *e2World) map[string]string {
			return map[string]string{"post_id": w.published.ID, "body": "a forged comment"}
		}},
	{http.MethodPost, "/controls/form/save_settings", e2Static("/controls/form/save_settings"),
		func(*e2World) map[string]string {
			return map[string]string{"timezone": "Europe/Berlin", "profile_visibility": "registered_users"}
		}},
	{http.MethodPost, "/controls/form/save_user_styles", e2Static("/controls/form/save_user_styles"),
		func(*e2World) map[string]string { return map[string]string{"styles": "body { color: red }"} }},
	{http.MethodPost, "/controls/form/change_password", e2Static("/controls/form/change_password"),
		func(*e2World) map[string]string {
			return map[string]string{"old_password": e2Password, "password": "a-new-password-123"}
		}},
	{http.MethodPost, "/controls/form/prompt_post", e2Static("/controls/form/prompt_post"),
		func(w *e2World) map[string]string {
			return map[string]string{"message": "write about it", "recipient_handle": w.friend.Username}
		}},
	{http.MethodPost, "/controls/form/add_user_feed", e2Static("/controls/form/add_user_feed"),
		func(*e2World) map[string]string { return map[string]string{"url": "http://127.0.0.1:1/feed.xml"} }},

	// /form
	{http.MethodPost, "/form/login", e2Static("/form/login"),
		func(w *e2World) map[string]string {
			return map[string]string{"email": w.stranger.Email, "password": e2Password}
		}},
	{http.MethodPost, "/form/accept_invite/:id", func(w *e2World) string { return "/form/accept_invite/" + w.invitation.ID },
		func(*e2World) map[string]string {
			return map[string]string{"username": "invitedguest", "password": "invited-password-1"}
		}},
	{http.MethodPost, "/form/signup", e2Static("/form/signup"),
		func(*e2World) map[string]string {
			return map[string]string{"email": "newcomer@example.com", "username": "newcomer", "password": "newcomer-password-1"}
		}},
	{http.MethodPost, "/form/signup_waiting_list", e2Static("/form/signup_waiting_list"),
		func(*e2World) map[string]string {
			return map[string]string{"email": "waiter@example.com", "reason": "curious"}
		}},
}

// e2Excluded are routes of the scanned files deliberately left out of the
// table, with the reason.
var e2Excluded = map[string]string{
	"GET /api/v1/posts":        "bearer-token API, owned by E4",
	"POST /api/v1/posts":       "bearer-token API, owned by E4",
	"POST /api/v1/posts/:id":   "bearer-token API, owned by E4",
	"DELETE /api/v1/posts/:id": "bearer-token API, owned by E4",
	"PUT /api/v1/image":        "bearer-token API, owned by E4",
}

var e2RouteRe = regexp.MustCompile(`\b(\w+)\.(GET|POST|PUT|DELETE)\("([^"]*)"(.*)$`)

// TestE2RouteTableMatchesSource builds the list of guarded routes from
// cmd/web and checks that e2Routes covers exactly those.
func TestE2RouteTableMatchesSource(t *testing.T) {
	t.Parallel()

	prefixes := map[string]map[string]string{
		"../cmd/web/main.go": {
			"router": "", "r": "", "apiGroup": "/api/v1", "controls": "/controls",
			"actions": "/controls/action", "nonControlsForms": "/form", "controlsForms": "/controls/form",
		},
		"../cmd/web/actions.go": {"r": "/controls/action"},
		"../cmd/web/api.go":     {"r": "/api/v1"},
	}

	want := map[string]bool{}

	for file, groups := range prefixes {
		src, err := os.ReadFile(file)
		require.NoError(t, err)

		for line := range strings.SplitSeq(string(src), "\n") {
			m := e2RouteRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			prefix, ok := groups[m[1]]
			require.Truef(t, ok, "%s: unknown router group %q in %q", file, m[1], strings.TrimSpace(line))

			path := m[3]
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}

			full := prefix + path
			if prefix != "" && path == "/" {
				full = prefix + "/"
			}

			key := m[2] + " " + full
			mutating := m[2] != http.MethodGet
			enforced := strings.Contains(m[4], "auth.EnforceAuth") || strings.HasPrefix(full, "/controls")

			if (mutating || enforced || strings.HasPrefix(full, "/api/")) && e2Excluded[key] == "" {
				want[key] = true
			}
		}
	}

	var have []string
	for _, r := range e2Routes {
		have = append(have, r.key())
	}

	var wantList []string
	for k := range want {
		wantList = append(wantList, k)
	}

	slices.Sort(have)
	slices.Sort(wantList)
	require.Equal(t, wantList, have)
}

// e2Raw sends a request outside e2e.Client, so the test controls the
// X-CSRFToken header exactly (the client always adds the right one).
func e2Raw(t *testing.T, app *e2e.App, cookies []*http.Cookie, method, path, contentType, body, csrfHeader string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, app.URL+path, strings.NewReader(body))
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if csrfHeader != "" {
		req.Header.Set("X-CSRFToken", csrfHeader)
	}

	for _, c := range cookies {
		req.AddCookie(c)
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	resp, err := client.Do(req)
	require.NoError(t, err)

	_, _ = io.Copy(io.Discard, resp.Body)
	require.NoError(t, resp.Body.Close())

	return resp
}

func e2Encode(t *testing.T, r e2Route, w *e2World, asForm bool, extra url.Values) (string, string) {
	t.Helper()

	payload := r.payload(w)

	if r.isJSON() && !asForm {
		b, err := json.Marshal(payload)
		require.NoError(t, err)

		return "application/json", string(b)
	}

	form := url.Values{}
	for k, v := range payload {
		form.Set(k, v)
	}

	maps.Copy(form, extra)

	return "application/x-www-form-urlencoded", form.Encode()
}

func e2Mutating(r e2Route) bool { return r.method != http.MethodGet }

// TestE2AnonymousRedirectsToLogin: every EnforceAuth route sends an anonymous
// visitor to /login with a signed return_url, and changes nothing.
func TestE2AnonymousRedirectsToLogin(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	w := e2NewWorld(t, app)
	before := w.snapshot(t)

	for _, r := range e2Routes {
		if strings.HasPrefix(r.pattern, "/form/") {
			continue // not behind EnforceAuth
		}

		t.Run(r.key(), func(t *testing.T) {
			anon := app.Client(t)
			path := r.path(w)

			var resp *e2e.Response

			switch {
			case !e2Mutating(r):
				resp = anon.Get(path)
			case r.isJSON():
				resp = anon.PostJSON(path, r.payload(w))
			default:
				form := url.Values{}
				for k, v := range r.payload(w) {
					form.Set(k, v)
				}

				resp = anon.PostForm(path, form)
			}

			resp.RequireStatus(http.StatusFound)

			loc, err := url.Parse(resp.Location())
			require.NoError(t, err)
			require.Equal(t, "/login", loc.Path)
			require.Equal(t, path, loc.Query().Get("return_url"))
			require.NotEmpty(t, loc.Query().Get("sign"))
		})
	}

	require.Equal(t, before, w.snapshot(t))
}

// TestE2CSRFRequired: every /form and /controls POST refuses a request whose
// CSRF token is missing or wrong, in the header or in the header_csrf field,
// and nothing changes.
func TestE2CSRFRequired(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	w := e2NewWorld(t, app)
	before := w.snapshot(t)

	type variant struct {
		name    string
		cookies []*http.Cookie
		asForm  bool
		header  string
		field   url.Values
	}

	for _, r := range e2Routes {
		if !e2Mutating(r) {
			continue
		}

		variants := []variant{
			{name: "missing", cookies: w.ownerSessionCookie},
			{name: "wrong header", cookies: w.ownerSessionCookie, header: "not-the-token"},
			{name: "wrong header_csrf field", cookies: w.ownerSessionCookie, asForm: true,
				field: url.Values{"header_csrf": {"not-the-token"}}},
			{name: "empty header_csrf field", cookies: w.ownerSessionCookie, asForm: true,
				field: url.Values{"header_csrf": {""}}},
		}

		if strings.HasPrefix(r.pattern, "/form/") {
			variants = append(variants,
				variant{name: "anonymous missing"},
				variant{name: "anonymous wrong header", header: "not-the-token"},
				variant{name: "anonymous wrong header_csrf field", asForm: true,
					field: url.Values{"header_csrf": {"not-the-token"}}},
			)
		}

		for _, v := range variants {
			t.Run(r.key()+"/"+v.name, func(t *testing.T) {
				ct, body := e2Encode(t, r, w, v.asForm, v.field)
				resp := e2Raw(t, app, v.cookies, r.method, r.path(w), ct, body, v.header)
				require.Equal(t, http.StatusForbidden, resp.StatusCode)
			})
		}
	}

	require.Equal(t, before, w.snapshot(t))
}

// TestE2CSRFTokenAccepted is the positive control for TestE2CSRFRequired:
// the same raw requests pass with the session's token, in the header or in
// the header_csrf field the settings page's export form carries.
func TestE2CSRFTokenAccepted(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	w := e2NewWorld(t, app)

	token, ok := w.ownerClient.Get("/controls/settings").RequireStatus(http.StatusOK).
		Doc().Find(`input[name="header_csrf"]`).First().Attr("value")
	require.True(t, ok)
	require.NotEmpty(t, token)

	resp := e2Raw(t, app, w.ownerSessionCookie, http.MethodPost, "/controls/action/generate_api_key",
		"application/json", "{}", token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = e2Raw(t, app, w.ownerSessionCookie, http.MethodPost, "/controls/action/settings/export",
		"application/x-www-form-urlencoded", url.Values{"header_csrf": {token}}.Encode(), "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))

	// the owner's token is useless in another session
	other := app.Client(t)
	other.LoginAs(w.friend.Email, e2Password)
	resp = e2Raw(t, app, other.Cookies(), http.MethodPost, "/controls/action/generate_api_key",
		"application/json", "{}", token)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestE2ForeignObjects: acting on objects that belong to someone else is
// refused and leaves the database unchanged.
func TestE2ForeignObjects(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	db := app.DB

	// e2Pair is an attacker, logged in, and a victim with their own session.
	type e2Pair struct {
		attacker, victim             *core.User
		attackerClient, victimClient *e2e.Client
	}

	newPair := func(t *testing.T) e2Pair {
		t.Helper()

		p := e2Pair{attacker: e2User(t, app), victim: e2User(t, app)}
		p.attackerClient = app.Client(t)
		p.attackerClient.LoginAs(p.attacker.Email, e2Password)
		p.victimClient = app.Client(t)
		p.victimClient.LoginAs(p.victim.Email, e2Password)

		return p
	}

	t.Run("edit someone else's post", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)

		for _, opts := range [][]factory.PostOpt{{factory.Published()}, nil} {
			post, err := factory.Post(ctx, db, p.victim.ID, opts...)
			require.NoError(t, err)

			p.attackerClient.PostForm("/controls/form/edit_post", url.Values{
				"post_id": {post.ID}, "subject": {"hijacked"}, "body": {"hijacked body"},
				"visibility": {string(core.PostVisibilityDirectOnly)}, "save_action": {"save_post"},
			}).RequireStatus(http.StatusNotFound)

			p.attackerClient.PostForm("/controls/form/edit_post", url.Values{
				"post_id": {post.ID}, "save_action": {"delete"},
			}).RequireStatus(http.StatusNotFound)

			got, err := factory.GetPost(ctx, db, post.ID)
			require.NoError(t, err)
			require.Equal(t, post.Subject, got.Subject)
			require.Equal(t, post.Body, got.Body)
			require.Equal(t, post.PublishedAt.Valid, got.PublishedAt.Valid)
			require.Equal(t, p.victim.ID, got.UserID)
		}

		mine, err := factory.ListPosts(ctx, db, p.attacker.ID)
		require.NoError(t, err)
		require.Empty(t, mine)
	})

	t.Run("delete someone else's draft", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		// even a direct connection may not delete the draft
		_, _, err := factory.Connect(ctx, db, p.attacker.ID, p.victim.ID)
		require.NoError(t, err)

		draft, err := factory.Post(ctx, db, p.victim.ID)
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/delete_draft", map[string]string{"postId": draft.ID}).
			RequireStatus(http.StatusBadRequest)

		got, err := factory.GetPost(ctx, db, draft.ID)
		require.NoError(t, err)
		require.Equal(t, draft.Body, got.Body)
	})

	t.Run("comment on a post the user cannot see", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		post, err := factory.Post(ctx, db, p.victim.ID, factory.Published())
		require.NoError(t, err)

		resp := p.attackerClient.PostForm("/controls/form/new_comment", url.Values{
			"post_id": {post.ID}, "body": {"sneaky comment"},
		})
		// the form guard renders the error inline rather than returning a status.
		require.Equal(t, http.StatusOK, resp.StatusCode)

		comments, err := factory.ListComments(ctx, db, post.ID)
		require.NoError(t, err)
		require.Empty(t, comments)
	})

	t.Run("reply to someone else's comment on another post", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		_, _, err := factory.Connect(ctx, db, p.attacker.ID, p.victim.ID)
		require.NoError(t, err)

		visible, err := factory.Post(ctx, db, p.victim.ID, factory.Published())
		require.NoError(t, err)

		stranger := e2User(t, app)
		hidden, err := factory.Post(ctx, db, stranger.ID, factory.Published())
		require.NoError(t, err)
		foreign, err := factory.Comment(ctx, db, hidden.ID, stranger.ID)
		require.NoError(t, err)

		resp := p.attackerClient.PostForm("/controls/form/new_comment", url.Values{
			"post_id": {visible.ID}, "body": {"threaded elsewhere"}, "reply_to": {foreign.ID},
		})
		// the form guard renders the error inline rather than returning a status.
		require.Equal(t, http.StatusOK, resp.StatusCode)

		comments, err := factory.ListComments(ctx, db, visible.ID)
		require.NoError(t, err)
		require.Empty(t, comments)

		comments, err = factory.ListComments(ctx, db, hidden.ID)
		require.NoError(t, err)
		require.Len(t, comments, 1)
	})

	t.Run("create a share for an unrelated user's post", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		post, err := factory.Post(ctx, db, p.victim.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/create_share", map[string]string{"postId": post.ID}).
			RequireStatus(http.StatusBadRequest)

		shared, err := factory.ShareExists(ctx, db, post.ID)
		require.NoError(t, err)
		require.False(t, shared)
	})

	t.Run("delete an unrelated user's share", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		post, err := factory.Post(ctx, db, p.victim.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
		require.NoError(t, err)
		share, err := factory.PostShare(ctx, db, post.ID)
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/delete_share", map[string]string{"postId": post.ID}).
			RequireStatus(http.StatusBadRequest)

		got, err := factory.GetPostShare(ctx, db, share.ID)
		require.NoError(t, err)
		require.Equal(t, post.ID, got.PostID)
	})

	t.Run("generating an API key leaves other users' keys alone", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		key, err := factory.APIKey(ctx, db, p.victim.ID)
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/generate_api_key", map[string]string{"userId": p.victim.ID}).
			RequireStatus(http.StatusOK)

		req, err := http.NewRequest(http.MethodGet, app.URL+"/api/v1/posts", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+key.APIKey)
		app.Client(t).Do(req).RequireStatus(http.StatusOK)

		attackerKeys, err := factory.ListAPIKeys(ctx, db, p.attacker.ID)
		require.NoError(t, err)
		require.Len(t, attackerKeys, 1)

		victimKeys, err := factory.ListAPIKeys(ctx, db, p.victim.ID)
		require.NoError(t, err)
		require.Len(t, victimKeys, 1)
		require.Equal(t, key.APIKey, victimKeys[0].APIKey)
	})

	t.Run("remove someone else's feed subscription", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		feed, err := factory.RSSFeed(ctx, db)
		require.NoError(t, err)
		sub, err := factory.Subscription(ctx, db, p.victim.ID, feed.ID)
		require.NoError(t, err)

		// Characterization: 200 for a no-op, since the delete is scoped to the caller and nothing changes.
		p.attackerClient.PostJSON("/controls/action/remove_rss_subscription", map[string]string{"id": sub.ID}).
			RequireStatus(http.StatusOK)

		exists, err := factory.SubscriptionExists(ctx, db, p.victim.ID, feed.ID)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("dismiss someone else's feed item", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		feed, err := factory.RSSFeed(ctx, db)
		require.NoError(t, err)
		_, err = factory.Subscription(ctx, db, p.victim.ID, feed.ID)
		require.NoError(t, err)
		item, err := factory.RSSItem(ctx, db, feed.ID)
		require.NoError(t, err)
		feedItem, err := factory.UserFeedItem(ctx, db, p.victim.ID, item.ID)
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/dissmiss_rss_item", map[string]string{"id": feedItem.ID}).
			RequireStatus(http.StatusBadRequest)

		got, err := factory.GetUserFeedItem(ctx, db, feedItem.ID)
		require.NoError(t, err)
		require.False(t, got.IsDismissed)
		require.Equal(t, p.victim.ID, got.UserID)
	})

	t.Run("dismiss someone else's prompt", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		asker := e2User(t, app)
		_, _, err := factory.Connect(ctx, db, asker.ID, p.victim.ID)
		require.NoError(t, err)
		prompt, err := factory.PostPrompt(ctx, db, asker.ID, p.victim.ID)
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/dismiss_prompt", map[string]string{"promptId": prompt.ID}).
			RequireStatus(http.StatusBadRequest)

		got, err := factory.GetPostPrompt(ctx, db, prompt.ID)
		require.NoError(t, err)
		require.False(t, got.DismissedAt.Valid)
	})

	t.Run("decide someone else's connection request", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		requester := e2User(t, app)
		req, err := factory.MediationRequest(ctx, db, requester.ID, p.victim.ID)
		require.NoError(t, err)

		for _, action := range []string{"accept_connection", "reject_connection"} {
			p.attackerClient.PostJSON("/controls/action/"+action, map[string]string{"requestId": req.ID}).
				RequireStatus(http.StatusBadRequest)
		}

		got, err := factory.GetMediationRequest(ctx, db, req.ID)
		require.NoError(t, err)
		require.False(t, got.TargetDecision.Valid)
		require.False(t, got.ConnectionID.Valid)

		for _, pair := range [][2]string{{requester.ID, p.victim.ID}, {requester.ID, p.attacker.ID}} {
			connected, err := factory.ConnectionExists(ctx, db, pair[0], pair[1])
			require.NoError(t, err)
			require.False(t, connected)
		}
	})

	t.Run("mediate a request between strangers", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		requester := e2User(t, app)
		req, err := factory.MediationRequest(ctx, db, requester.ID, p.victim.ID)
		require.NoError(t, err)

		for _, action := range []string{"sign_mediation", "dismiss_mediation"} {
			p.attackerClient.PostJSON("/controls/action/"+action, map[string]string{"requestId": req.ID}).
				RequireStatus(http.StatusBadRequest)
		}

		got, err := factory.GetMediationRequest(ctx, db, req.ID)
		require.NoError(t, err)
		require.False(t, got.TargetDecision.Valid)

		decisions, err := factory.ListMediatorDecisions(ctx, db, req.ID)
		require.NoError(t, err)
		require.Empty(t, decisions)
	})

	t.Run("revoke someone else's mediation request", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		requester := e2User(t, app)
		req, err := factory.MediationRequest(ctx, db, requester.ID, p.victim.ID)
		require.NoError(t, err)

		p.attackerClient.PostJSON("/controls/action/revoke_mediation_request", map[string]string{"userId": p.victim.ID}).
			RequireStatus(http.StatusBadRequest)

		_, err = factory.GetMediationRequest(ctx, db, req.ID)
		require.NoError(t, err)
	})

	t.Run("drop someone else's connection", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		other := e2User(t, app)
		_, _, err := factory.Connect(ctx, db, p.victim.ID, other.ID)
		require.NoError(t, err)

		// Characterization: 200 for a no-op, since the caller has no such connection and nothing changes.
		p.attackerClient.PostJSON("/controls/action/drop_connection", map[string]string{"userId": other.ID}).
			RequireStatus(http.StatusOK)

		connected, err := factory.ConnectionExists(ctx, db, p.victim.ID, other.ID)
		require.NoError(t, err)
		require.True(t, connected)
	})

	t.Run("remove someone else's whitelist entry", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)
		other := e2User(t, app)
		_, err := factory.Whitelist(ctx, db, p.victim.ID, other.ID)
		require.NoError(t, err)

		// Characterization: 200 for a no-op, since the delete is scoped to the caller's grants and nothing changes.
		p.attackerClient.PostJSON("/controls/action/remove_from_whitelist", map[string]string{"userId": other.ID}).
			RequireStatus(http.StatusOK)

		exists, err := factory.WhitelistExists(ctx, db, p.victim.ID, other.ID)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("connect without a grant", func(t *testing.T) {
		t.Parallel()

		p := newPair(t)

		p.attackerClient.PostJSON("/controls/action/create_connection", map[string]string{"userId": p.victim.ID}).
			RequireStatus(http.StatusBadRequest)

		connected, err := factory.ConnectionExists(ctx, db, p.attacker.ID, p.victim.ID)
		require.NoError(t, err)
		require.False(t, connected)
	})
}

// TestE2LoginWhileLoggedIn: the login and signup pages send a logged-in user
// home.
func TestE2LoginWhileLoggedIn(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := e2User(t, app)
	client := app.Client(t)
	client.LoginAs(user.Email, e2Password)

	for _, path := range []string{"/login", "/login?return_url=%2Fwrite&sign=x", "/signup"} {
		resp := client.Get(path).RequireStatus(http.StatusFound)
		require.Equal(t, "/feed", resp.Location(), path)
	}
}

// TestE2LoginReturnURL: /login keeps a return_url signed by the server and
// drops one whose signature is missing or wrong.
func TestE2LoginReturnURL(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	client := app.Client(t)

	signed := client.Get("/write").RequireStatus(http.StatusFound).Location()
	loc, err := url.Parse(signed)
	require.NoError(t, err)
	sign := loc.Query().Get("sign")
	require.NotEmpty(t, sign)

	hidden := func(t *testing.T, path string) (string, string) {
		t.Helper()

		doc := client.Get(path).RequireStatus(http.StatusOK).Doc()
		ret, _ := doc.Find(`input[name="return_url"]`).Attr("value")
		sig, _ := doc.Find(`input[name="sign"]`).Attr("value")

		return ret, sig
	}

	ret, sig := hidden(t, signed)
	require.Equal(t, "/write", ret)
	require.Equal(t, sign, sig)

	for _, path := range []string{
		"/login?" + url.Values{"return_url": {"/controls/settings"}, "sign": {sign}}.Encode(),
		"/login?" + url.Values{"return_url": {"/write"}, "sign": {"forged"}}.Encode(),
		"/login?" + url.Values{"return_url": {"/write"}}.Encode(),
		"/login?" + url.Values{"return_url": {"https://evil.example/"}, "sign": {sign}}.Encode(),
	} {
		ret, sig := hidden(t, path)
		require.Empty(t, ret, path)
		require.Empty(t, sig, path)
	}
}

// TestE2LoginBadCredentials: a wrong password, an unknown email and an
// unconfirmed account all fail to log in.
func TestE2LoginBadCredentials(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	unconfirmed, err := factory.User(ctx, app.DB, factory.WithPassword(e2Password), func(u *core.User) {
		u.EmailConfirmedAt = null.Time{}
	})
	require.NoError(t, err)

	for _, creds := range [][2]string{
		{"nobody@example.test", e2Password},
		{unconfirmed.Email, e2Password},
	} {
		client := app.Client(t)
		client.Get("/login").RequireStatus(http.StatusOK)

		resp := client.PostForm("/form/login", url.Values{"email": {creds[0]}, "password": {creds[1]}}).
			RequireStatus(http.StatusOK)
		require.Contains(t, resp.Doc().Find(".alert-danger").Text(), "Bad credentials", creds[0])

		client.Get("/feed").RequireStatus(http.StatusFound)
	}
}

// TestE2LoginCaseInsensitiveEmail: logging in with the account's email
// upper-cased, and the correct password, should succeed. Today the lookup is
// case-sensitive and the attempt is refused as bad credentials.
func TestE2LoginCaseInsensitiveEmail(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/114")
	t.Parallel()

	app := e2e.Start(t)
	user := e2User(t, app)

	// a fresh, never-logged-in client is sent to /login from /feed, so a 200
	// below proves the session the form login established, not a default.
	app.Client(t).Get("/feed").RequireStatus(http.StatusFound)

	client := app.Client(t)
	client.Get("/login").RequireStatus(http.StatusOK)

	client.PostForm("/form/login", url.Values{
		"email": {strings.ToUpper(user.Email)}, "password": {e2Password},
	}).RequireStatus(http.StatusOK)

	client.Get("/feed").RequireStatus(http.StatusOK)
}

// The #109 tests: a logged-in user hitting a guest-only route is redirected
// home and the handler has no side effect.

func e2LoggedIn(t *testing.T, app *e2e.App) (*core.User, *e2e.Client) {
	t.Helper()

	user := e2User(t, app)
	client := app.Client(t)
	client.LoginAs(user.Email, e2Password)

	return user, client
}

func TestE2LoggedInConfirmSignupHasNoEffect(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/109")
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	_, client := e2LoggedIn(t, app)

	pending, err := factory.User(ctx, app.DB, func(u *core.User) {
		u.EmailConfirmedAt = null.Time{}
		u.EmailConfirmSeed = null.StringFrom("seed-" + u.Username)
	})
	require.NoError(t, err)

	resp := client.Get("/confirm_signup/" + pending.EmailConfirmSeed.String).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	got, err := factory.GetUser(ctx, app.DB, pending.ID)
	require.NoError(t, err)
	require.False(t, got.EmailConfirmedAt.Valid)

	emails, err := factory.ListOutgoingEmails(ctx, app.DB, core.OutgoingEmailWhere.EmailType.EQ("signup_confirmed"))
	require.NoError(t, err)
	require.Empty(t, emails)
}

func TestE2LoggedInConfirmWaitingListHasNoEffect(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/109")
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	_, client := e2LoggedIn(t, app)

	request, err := factory.SignupRequest(ctx, app.DB)
	require.NoError(t, err)

	resp := client.Get("/confirm_waiting_list/" + request.ID).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	got, err := factory.GetSignupRequest(ctx, app.DB, request.ID)
	require.NoError(t, err)
	require.False(t, got.EmailConfirmedAt.Valid)
}

func TestE2LoggedInFormLoginHasNoEffect(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/109")
	t.Parallel()

	app := e2e.Start(t)
	user, client := e2LoggedIn(t, app)
	other := e2User(t, app)

	resp := client.PostForm("/form/login", url.Values{"email": {other.Email}, "password": {e2Password}}).
		RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	require.Equal(t, user.Username, e2CurrentUsername(t, client))
}

func TestE2LoggedInFormSignupHasNoEffect(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/109")
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	require.NoError(t, factory.SetRegistrationOpen(ctx, app.DB, true))
	user, client := e2LoggedIn(t, app)

	resp := client.PostForm("/form/signup", url.Values{
		"email": {"newcomer@example.com"}, "username": {"newcomer"}, "password": {"newcomer-password-1"},
	}).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	emails, err := factory.ListOutgoingEmails(ctx, app.DB)
	require.NoError(t, err)
	require.Empty(t, emails)

	require.Equal(t, user.Username, e2CurrentUsername(t, client))
	app.Client(t).Get("/users/newcomer").RequireStatus(http.StatusNotFound)
}

func TestE2LoggedInFormSignupWaitingListHasNoEffect(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/109")
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	_, client := e2LoggedIn(t, app)

	resp := client.PostForm("/form/signup_waiting_list", url.Values{
		"email": {"waiter@example.com"}, "reason": {"curious"},
	}).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	emails, err := factory.ListOutgoingEmails(ctx, app.DB)
	require.NoError(t, err)
	require.Empty(t, emails)

	exists, err := factory.SignupRequestExists(ctx, app.DB, "waiter@example.com")
	require.NoError(t, err)
	require.False(t, exists)
}
