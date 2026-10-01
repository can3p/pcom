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
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// guardWorld is the fixture every guard test acts on: an owner who is logged in,
// plus the objects the mutating routes could touch.
type guardWorld struct {
	app *e2e.App

	owner, friend, stranger, requester *core.User

	// the addresses the send_invite, signup and signup_waiting_list routes
	// take, unique to the world: every test shares one tommy
	inviteeEmail, newcomerEmail, waiterEmail string

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

func newGuardWorld(t *testing.T, app *e2e.App) *guardWorld {
	t.Helper()

	ctx := context.Background()
	db := app.DB
	w := &guardWorld{
		app:           app,
		inviteeEmail:  uniqueEmail("invitee"),
		newcomerEmail: uniqueEmail("newcomer"),
		waiterEmail:   uniqueEmail("waiter"),
	}

	var err error

	w.owner = newUser(t, app)
	w.friend = newUser(t, app)
	w.stranger = newUser(t, app)
	w.requester = newUser(t, app)

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
	w.ownerClient.LoginAs(w.owner.Email)
	w.ownerSessionCookie = w.ownerClient.Cookies()

	return w
}

// dbSnapshot is the part of the world a mutating route could change.
type dbSnapshot struct {
	Users      []string
	Posts      []string
	Comments   []string
	Emails     int
	Queued     int
	Connected  bool
	Request    string
	Mediators  int
	Subscribed bool
	ItemHidden bool
	Prompt     bool
	LoggedInAs string
}

// isWorldMail matches the mail a route could send in this world: mail
// linking to this app, mail to the world's users or to the addresses the
// routes take, and admin notices naming one of those. Mail from an earlier
// app on the same port also links to it, but it is counted before and after
// alike.
func (w *guardWorld) isWorldMail(m tommy.Mail) bool {
	link := w.app.URL + "/"
	if strings.Contains(m.Text, link) || strings.Contains(m.HTML, link) {
		return true
	}

	for _, addr := range []string{
		w.owner.Email, w.friend.Email, w.stranger.Email, w.requester.Email,
		w.inviteeEmail, w.newcomerEmail, w.waiterEmail,
	} {
		if m.SentTo(addr) || strings.Contains(m.Text, addr) || strings.Contains(m.HTML, addr) {
			return true
		}
	}

	return false
}

// uniqueEmail is an address no other test uses.
func uniqueEmail(name string) string {
	return name + "-" + uuid.NewString() + "@example.test"
}

func (w *guardWorld) snapshot(t *testing.T) dbSnapshot {
	t.Helper()

	ctx := context.Background()
	db := w.app.DB

	var s dbSnapshot

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

	s.Emails = len(w.app.SentMails(t, "", w.isWorldMail))
	s.Queued = w.app.QueuedMails(t)

	var err error

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

	s.LoggedInAs = currentUsername(t, w.ownerClient)

	return s
}

// currentUsername is the username the navbar greets on /feed, or "" when the
// session is anonymous.
func currentUsername(t *testing.T, c *e2e.Client) string {
	t.Helper()

	resp := c.Get("/feed")
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	return strings.TrimSpace(resp.Doc().Find(".navbar-text a").First().Text())
}

// guardRoute is one mutating or EnforceAuth route, as registered in cmd/web.
type guardRoute struct {
	method  string
	pattern string
	// path returns the concrete path for the pattern.
	path func(w *guardWorld) string
	// payload is what a legitimate request would send, so that a guard that
	// let the request through would visibly change the database.
	payload func(w *guardWorld) map[string]string
}

func (r guardRoute) key() string { return r.method + " " + r.pattern }

func (r guardRoute) isJSON() bool {
	return strings.HasPrefix(r.pattern, "/controls/action/")
}

func staticPath(p string) func(*guardWorld) string { return func(*guardWorld) string { return p } }

func noExtra(*guardWorld) map[string]string { return nil }

// guardRoutes is every POST/PUT/DELETE and EnforceAuth route of cmd/web outside
// /api/v1. TestGuards_RouteTableMatchesSource keeps it in sync with the source.
var guardRoutes = []guardRoute{
	// EnforceAuth GET routes
	{http.MethodGet, "/posts/:id/edit", func(w *guardWorld) string { return "/posts/" + w.draft.ID + "/edit" }, noExtra},
	{http.MethodGet, "/write", staticPath("/write"), noExtra},
	{http.MethodGet, "/feed", staticPath("/feed"), noExtra},
	{http.MethodGet, "/controls/", staticPath("/controls/"), noExtra},
	{http.MethodGet, "/controls/settings", staticPath("/controls/settings"), noExtra},

	// /controls/action (actions.go and logout)
	{http.MethodPost, "/controls/action/logout", staticPath("/controls/action/logout"), noExtra},
	{http.MethodPost, "/controls/action/remove_from_whitelist", staticPath("/controls/action/remove_from_whitelist"),
		func(w *guardWorld) map[string]string { return map[string]string{"userId": w.stranger.ID} }},
	{http.MethodPost, "/controls/action/create_connection", staticPath("/controls/action/create_connection"),
		func(w *guardWorld) map[string]string { return map[string]string{"userId": w.stranger.ID} }},
	{http.MethodPost, "/controls/action/drop_connection", staticPath("/controls/action/drop_connection"),
		func(w *guardWorld) map[string]string { return map[string]string{"userId": w.friend.ID} }},
	{http.MethodPost, "/controls/action/request_mediation", staticPath("/controls/action/request_mediation"),
		func(w *guardWorld) map[string]string {
			return map[string]string{"userId": w.stranger.ID, "mediation_note": "hi"}
		}},
	{http.MethodPost, "/controls/action/revoke_mediation_request", staticPath("/controls/action/revoke_mediation_request"),
		func(w *guardWorld) map[string]string { return map[string]string{"userId": w.stranger.ID} }},
	{http.MethodPost, "/controls/action/dismiss_mediation", staticPath("/controls/action/dismiss_mediation"),
		func(w *guardWorld) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/sign_mediation", staticPath("/controls/action/sign_mediation"),
		func(w *guardWorld) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/reject_connection", staticPath("/controls/action/reject_connection"),
		func(w *guardWorld) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/accept_connection", staticPath("/controls/action/accept_connection"),
		func(w *guardWorld) map[string]string { return map[string]string{"requestId": w.request.ID} }},
	{http.MethodPost, "/controls/action/delete_draft", staticPath("/controls/action/delete_draft"),
		func(w *guardWorld) map[string]string { return map[string]string{"postId": w.draft.ID} }},
	{http.MethodPost, "/controls/action/generate_api_key", staticPath("/controls/action/generate_api_key"), noExtra},
	{http.MethodPost, "/controls/action/regenerate_feed_token", staticPath("/controls/action/regenerate_feed_token"), noExtra},
	{http.MethodPost, "/controls/action/dismiss_prompt", staticPath("/controls/action/dismiss_prompt"),
		func(w *guardWorld) map[string]string { return map[string]string{"promptId": w.prompt.ID} }},
	{http.MethodPost, "/controls/action/remove_rss_subscription", staticPath("/controls/action/remove_rss_subscription"),
		func(w *guardWorld) map[string]string { return map[string]string{"id": w.subscription.ID} }},
	{http.MethodPost, "/controls/action/dissmiss_rss_item", staticPath("/controls/action/dissmiss_rss_item"),
		func(w *guardWorld) map[string]string { return map[string]string{"id": w.feedItem.ID} }},
	{http.MethodPost, "/controls/action/create_share", staticPath("/controls/action/create_share"),
		func(w *guardWorld) map[string]string { return map[string]string{"postId": w.published.ID} }},
	{http.MethodPost, "/controls/action/delete_share", staticPath("/controls/action/delete_share"),
		func(w *guardWorld) map[string]string { return map[string]string{"postId": w.published.ID} }},
	{http.MethodPost, "/controls/action/upload_media", staticPath("/controls/action/upload_media"), noExtra},
	{http.MethodPost, "/controls/action/settings/export", staticPath("/controls/action/settings/export"), noExtra},
	{http.MethodPost, "/controls/action/settings/import", staticPath("/controls/action/settings/import"), noExtra},

	// /controls/form
	{http.MethodPost, "/controls/form/whitelist_connection", staticPath("/controls/form/whitelist_connection"),
		func(w *guardWorld) map[string]string { return map[string]string{"uname": w.stranger.Username} }},
	{http.MethodPost, "/controls/form/send_invite", staticPath("/controls/form/send_invite"),
		func(w *guardWorld) map[string]string { return map[string]string{"email": w.inviteeEmail} }},
	{http.MethodPost, "/controls/form/edit_post", staticPath("/controls/form/edit_post"),
		func(w *guardWorld) map[string]string {
			return map[string]string{
				"post_id": w.published.ID, "subject": "hijacked", "body": "hijacked body",
				"visibility": string(core.PostVisibilityDirectOnly), "save_action": "save_post",
			}
		}},
	{http.MethodPost, "/controls/form/new_comment", staticPath("/controls/form/new_comment"),
		func(w *guardWorld) map[string]string {
			return map[string]string{"post_id": w.published.ID, "body": "a forged comment"}
		}},
	{http.MethodPost, "/controls/form/edit_comment/:id", func(w *guardWorld) string { return "/controls/form/edit_comment/" + w.comment.ID },
		func(*guardWorld) map[string]string { return map[string]string{"body": "a forged edit"} }},
	{http.MethodPost, "/controls/form/save_settings", staticPath("/controls/form/save_settings"),
		func(*guardWorld) map[string]string {
			return map[string]string{"timezone": "Europe/Berlin", "profile_visibility": "registered_users"}
		}},
	{http.MethodPost, "/controls/form/save_user_styles", staticPath("/controls/form/save_user_styles"),
		func(*guardWorld) map[string]string { return map[string]string{"styles": "body { color: red }"} }},
	{http.MethodPost, "/controls/form/save_profile", staticPath("/controls/form/save_profile"),
		func(*guardWorld) map[string]string { return map[string]string{"about": "About me"} }},
	{http.MethodPost, "/controls/form/change_password", staticPath("/controls/form/change_password"),
		func(*guardWorld) map[string]string {
			return map[string]string{"old_password": testPassword, "password": "a-new-password-123"}
		}},
	{http.MethodPost, "/controls/form/prompt_post", staticPath("/controls/form/prompt_post"),
		func(w *guardWorld) map[string]string {
			return map[string]string{"message": "write about it", "recipient_handle": w.friend.Username}
		}},
	{http.MethodPost, "/controls/form/add_user_feed", staticPath("/controls/form/add_user_feed"),
		func(*guardWorld) map[string]string { return map[string]string{"url": "http://127.0.0.1:1/feed.xml"} }},

	// /form
	{http.MethodPost, "/form/login", staticPath("/form/login"),
		func(w *guardWorld) map[string]string {
			return map[string]string{"email": w.stranger.Email}
		}},
	{http.MethodPost, "/form/login/code", staticPath("/form/login/code"),
		func(*guardWorld) map[string]string { return map[string]string{"code": "123456"} }},
	{http.MethodPost, "/form/accept_invite/:id", func(w *guardWorld) string { return "/form/accept_invite/" + w.invitation.ID },
		func(*guardWorld) map[string]string {
			return map[string]string{"username": "invitedguest", "password": "invited-password-1"}
		}},
	{http.MethodPost, "/form/signup", staticPath("/form/signup"),
		func(w *guardWorld) map[string]string {
			return map[string]string{"email": w.newcomerEmail, "username": "newcomer", "password": "newcomer-password-1"}
		}},
	{http.MethodPost, "/form/signup_waiting_list", staticPath("/form/signup_waiting_list"),
		func(w *guardWorld) map[string]string {
			return map[string]string{"email": w.waiterEmail, "reason": "curious"}
		}},
}

// excludedRoutes are routes of the scanned files deliberately left out of the
// table, with the reason.
var excludedRoutes = map[string]string{
	"GET /api/v1/posts":        "bearer-token API, covered in api_test.go",
	"POST /api/v1/posts":       "bearer-token API, covered in api_test.go",
	"POST /api/v1/posts/:id":   "bearer-token API, covered in api_test.go",
	"DELETE /api/v1/posts/:id": "bearer-token API, covered in api_test.go",
	"PUT /api/v1/image":        "bearer-token API, covered in api_test.go",
}

var routeRe = regexp.MustCompile(`\b(\w+)\.(GET|POST|PUT|DELETE)\("([^"]*)"(.*)$`)

// TestGuards_RouteTableMatchesSource builds the list of guarded routes from
// pkg/web/app and checks that guardRoutes covers exactly those.
func TestGuards_RouteTableMatchesSource(t *testing.T) {
	t.Parallel()

	prefixes := map[string]map[string]string{
		"../pkg/web/app/routes_media.go":        {"router": ""},
		"../pkg/web/app/routes_public.go":       {"r": ""},
		"../pkg/web/app/routes_rss.go":          {"r": ""},
		"../pkg/web/app/routes_auth.go":         {"r": "", "actions": "/controls/action", "nonControlsForms": "/form"},
		"../pkg/web/app/routes_connections.go":  {"controls": "/controls", "controlsForms": "/controls/form"},
		"../pkg/web/app/routes_posts.go":        {"r": "", "controlsForms": "/controls/form"},
		"../pkg/web/app/routes_settings.go":     {"r": "", "controls": "/controls", "controlsForms": "/controls/form"},
		"../pkg/web/app/routes_feeds.go":        {"controlsForms": "/controls/form"},
		"../pkg/web/app/actions_export.go":      {"r": "/controls/action"},
		"../pkg/web/app/actions_connections.go": {"r": "/controls/action"},
		"../pkg/web/app/actions_mediation.go":   {"r": "/controls/action"},
		"../pkg/web/app/actions_posts.go":       {"r": "/controls/action"},
		"../pkg/web/app/actions_shares.go":      {"r": "/controls/action"},
		"../pkg/web/app/actions_prompts.go":     {"r": "/controls/action"},
		"../pkg/web/app/actions_rss.go":         {"r": "/controls/action"},
		"../pkg/web/app/actions_settings.go":    {"r": "/controls/action"},
		"../pkg/web/app/actions_media.go":       {"r": "/controls/action"},
		"../pkg/web/app/api.go":                 {"r": "/api/v1"},
	}

	want := map[string]bool{}

	for file, groups := range prefixes {
		src, err := os.ReadFile(file)
		require.NoError(t, err)

		for line := range strings.SplitSeq(string(src), "\n") {
			m := routeRe.FindStringSubmatch(line)
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

			if (mutating || enforced || strings.HasPrefix(full, "/api/")) && excludedRoutes[key] == "" {
				want[key] = true
			}
		}
	}

	var have []string
	for _, r := range guardRoutes {
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

// rawRequest sends a request outside e2e.Client, so the test controls the
// X-CSRFToken header exactly (the client always adds the right one).
func rawRequest(t *testing.T, app *e2e.App, cookies []*http.Cookie, method, path, contentType, body, csrfHeader string) *http.Response {
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

func encodeBody(t *testing.T, r guardRoute, w *guardWorld, asForm bool, extra url.Values) (string, string) {
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

func isMutating(r guardRoute) bool { return r.method != http.MethodGet }

// TestGuards_AnonymousRedirectsToLogin: every EnforceAuth route sends an anonymous
// visitor to /login with a signed return_url, and changes nothing.
func TestGuards_AnonymousRedirectsToLogin(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	w := newGuardWorld(t, app)
	before := w.snapshot(t)

	for _, r := range guardRoutes {
		if strings.HasPrefix(r.pattern, "/form/") {
			continue // not behind EnforceAuth
		}

		t.Run(r.key(), func(t *testing.T) {
			anon := app.Client(t)
			path := r.path(w)

			var resp *e2e.Response

			switch {
			case !isMutating(r):
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

// TestGuards_CSRFRequired: every /form and /controls POST refuses a request whose
// CSRF token is missing or wrong, in the header or in the header_csrf field,
// and nothing changes.
func TestGuards_CSRFRequired(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	w := newGuardWorld(t, app)
	before := w.snapshot(t)

	type variant struct {
		name    string
		cookies []*http.Cookie
		asForm  bool
		header  string
		field   url.Values
	}

	for _, r := range guardRoutes {
		if !isMutating(r) {
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
				ct, body := encodeBody(t, r, w, v.asForm, v.field)
				resp := rawRequest(t, app, v.cookies, r.method, r.path(w), ct, body, v.header)
				require.Equal(t, http.StatusForbidden, resp.StatusCode)
			})
		}
	}

	require.Equal(t, before, w.snapshot(t))
}

// TestGuards_CSRFTokenAccepted is the positive control for TestGuards_CSRFRequired:
// the same raw requests pass with the session's token, in the header or in
// the header_csrf field the settings page's export form carries.
func TestGuards_CSRFTokenAccepted(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	w := newGuardWorld(t, app)

	token, ok := w.ownerClient.Get("/controls/settings").RequireStatus(http.StatusOK).
		Doc().Find(`input[name="header_csrf"]`).First().Attr("value")
	require.True(t, ok)
	require.NotEmpty(t, token)

	resp := rawRequest(t, app, w.ownerSessionCookie, http.MethodPost, "/controls/action/generate_api_key",
		"application/json", "{}", token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = rawRequest(t, app, w.ownerSessionCookie, http.MethodPost, "/controls/action/settings/export",
		"application/x-www-form-urlencoded", url.Values{"header_csrf": {token}}.Encode(), "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))

	// the owner's token is useless in another session
	other := app.Client(t)
	other.LoginAs(w.friend.Email)
	resp = rawRequest(t, app, other.Cookies(), http.MethodPost, "/controls/action/generate_api_key",
		"application/json", "{}", token)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestGuards_ForeignObjects: acting on objects that belong to someone else is
// refused and leaves the database unchanged.
func TestGuards_ForeignObjects(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	db := app.DB

	// attackerPair is an attacker, logged in, and a victim with their own session.
	type attackerPair struct {
		attacker, victim             *core.User
		attackerClient, victimClient *e2e.Client
	}

	newPair := func(t *testing.T) attackerPair {
		t.Helper()

		p := attackerPair{attacker: newUser(t, app), victim: newUser(t, app)}
		p.attackerClient = app.Client(t)
		p.attackerClient.LoginAs(p.attacker.Email)
		p.victimClient = app.Client(t)
		p.victimClient.LoginAs(p.victim.Email)

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

		stranger := newUser(t, app)
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
		asker := newUser(t, app)
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
		requester := newUser(t, app)
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
		requester := newUser(t, app)
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
		requester := newUser(t, app)
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
		other := newUser(t, app)
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
		other := newUser(t, app)
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

// TestGuards_LoginWhileLoggedIn: the login and signup pages send a logged-in user
// home.
func TestGuards_LoginWhileLoggedIn(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := newUser(t, app)
	client := app.Client(t)
	client.LoginAs(user.Email)

	for _, path := range []string{"/login", "/login?return_url=%2Fwrite&sign=x", "/signup"} {
		resp := client.Get(path).RequireStatus(http.StatusFound)
		require.Equal(t, "/feed", resp.Location(), path)
	}
}

// TestGuards_LoginReturnURL: /login keeps a return_url signed by the server and
// drops one whose signature is missing or wrong.
func TestGuards_LoginReturnURL(t *testing.T) {
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

	// the signed return url survives the code step; the code lands there
	user := newUser(t, app)
	step := client.PostForm("/form/login", url.Values{"email": {user.Email}, "return_url": {"/write"}, "sign": {sign}}).
		RequireStatus(http.StatusOK)

	// "Use a different email" goes back to the email step with it kept
	back, ok := step.Doc().Find("a:contains('Use a different email')").Attr("href")
	require.True(t, ok)
	require.Equal(t, "/login?"+url.Values{"return_url": {"/write"}, "sign": {sign}}.Encode(), back)
	ret, sig = hidden(t, back)
	require.Equal(t, "/write", ret)
	require.Equal(t, sign, sig)

	done := client.PostForm("/form/login/code", url.Values{"code": {app.IssueLoginCode(t, user.Email)}}).
		RequireStatus(http.StatusOK)
	require.Equal(t, app.URL+"/write", done.Header.Get("HX-Redirect"))

	// a forged signature is dropped at the first step
	other := newUser(t, app)
	forged := app.Client(t)
	forged.PostForm("/form/login", url.Values{"email": {other.Email}, "return_url": {"/write"}, "sign": {"forged"}}).
		RequireStatus(http.StatusOK)
	done = forged.PostForm("/form/login/code", url.Values{"code": {app.IssueLoginCode(t, other.Email)}}).
		RequireStatus(http.StatusOK)
	require.Equal(t, "/feed", done.Header.Get("HX-Redirect"))
}

// TestGuards_LoginUnknownAddressGetsSamePageAndNoMail: an unknown and an
// unconfirmed address are answered with the code form like a known one, and
// no mail goes to them, so the page tells nothing about who has an account.
func TestGuards_LoginUnknownAddressGetsSamePageAndNoMail(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	unconfirmed, err := factory.User(ctx, app.DB, func(u *core.User) {
		u.EmailConfirmedAt = null.Time{}
	})
	require.NoError(t, err)

	known := newUser(t, app)

	for _, email := range []string{"nobody@example.test", unconfirmed.Email, known.Email} {
		client := app.Client(t)
		client.Get("/login").RequireStatus(http.StatusOK)

		resp := client.PostForm("/form/login", url.Values{"email": {email}}).RequireStatus(http.StatusOK)
		require.Equal(t, 1, resp.Doc().Find(`input[name="code"]`).Length(), email)
		require.Zero(t, resp.Doc().Find(".alert-danger").Length(), email)

		client.Get("/feed").RequireStatus(http.StatusFound)
	}

	app.NoMails(t, "nobody@example.test", nil)
	app.NoMails(t, unconfirmed.Email, nil)
	require.Len(t, app.Mails(t, known.Email, nil), 1)
}

// TestGuards_LoginRotatesSession pins #122: the session cookie a visitor
// holds before logging in is replaced at login, and the old one no longer
// carries any session.
func TestGuards_LoginRotatesSession(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := newUser(t, app)

	sessCookie := func(c *e2e.Client) *http.Cookie {
		for _, ck := range c.Cookies() {
			if ck.Name == "sess" {
				return ck
			}
		}

		t.Fatal("no session cookie")

		return nil
	}

	client := app.Client(t)
	client.Get("/login").RequireStatus(http.StatusOK)
	before := sessCookie(client)

	client.LoginAs(user.Email)
	require.NotEqual(t, before.Value, sessCookie(client).Value)

	planted := app.Client(t)
	req, err := http.NewRequest(http.MethodGet, app.URL+"/feed", nil)
	require.NoError(t, err)
	req.AddCookie(before)
	planted.Do(req).RequireStatus(http.StatusFound)
}

// The #109 tests: a logged-in user hitting a guest-only route is redirected
// home and the handler has no side effect.

func TestGuards_LoggedInConfirmWaitingListHasNoEffect(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	_, client := newLoggedIn(t, app)

	request, err := factory.SignupRequest(ctx, app.DB)
	require.NoError(t, err)

	resp := client.Get("/confirm_waiting_list/" + request.ID).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	got, err := factory.GetSignupRequest(ctx, app.DB, request.ID)
	require.NoError(t, err)
	require.False(t, got.EmailConfirmedAt.Valid)
}

func TestGuards_LoggedInFormLoginHasNoEffect(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user, client := newLoggedIn(t, app)
	other := newUser(t, app)

	resp := client.PostForm("/form/login", url.Values{"email": {other.Email}}).
		RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	require.Equal(t, user.Username, currentUsername(t, client))
}

func TestGuards_LoggedInFormSignupHasNoEffect(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	require.NoError(t, factory.SetRegistrationOpen(ctx, app.DB, true))
	user, client := newLoggedIn(t, app)
	email := uniqueEmail("newcomer")

	resp := client.PostForm("/form/signup", url.Values{
		"email": {email}, "username": {"newcomer"}, "password": {"newcomer-password-1"},
	}).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	app.NoMails(t, "", mentions(email))

	require.Equal(t, user.Username, currentUsername(t, client))
	app.Client(t).Get("/users/newcomer").RequireStatus(http.StatusNotFound)
}

// mentions matches the mail a signup or waiting list request from the
// unique address addr could send: to addr itself, or an admin notice naming it.
func mentions(addr string) func(tommy.Mail) bool {
	return func(m tommy.Mail) bool {
		return m.SentTo(addr) || strings.Contains(m.Text, addr) || strings.Contains(m.HTML, addr)
	}
}

func TestGuards_LoggedInFormSignupWaitingListHasNoEffect(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	_, client := newLoggedIn(t, app)
	email := uniqueEmail("waiter")

	resp := client.PostForm("/form/signup_waiting_list", url.Values{
		"email": {email}, "reason": {"curious"},
	}).RequireStatus(http.StatusFound)
	require.Equal(t, "/feed", resp.Location())

	app.NoMails(t, "", mentions(email))

	exists, err := factory.SignupRequestExists(ctx, app.DB, email)
	require.NoError(t, err)
	require.False(t, exists)
}
