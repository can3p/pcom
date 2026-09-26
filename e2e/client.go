package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Client is a plain HTTP client for one App, for testing server rules that
// don't depend on the frontend: access control, status codes, the API, RSS,
// security headers. It keeps cookies, does not follow redirects (tests assert
// on them), and sends the CSRF token the pages carry.
//
// It deliberately does not imitate htmx: no HX-Request header, and Response
// has no accessors for HX-* response headers. Anything that depends on htmx
// or the page's JavaScript is tested in a real browser (e2e/browser), where
// an htmx upgrade that breaks the page fails the test.
type Client struct {
	t    testing.TB
	app  *App
	http *http.Client
	csrf string
}

// Client returns a new client with an empty cookie jar, so an anonymous
// visitor until LoginAs.
func (a *App) Client(t testing.TB) *Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	return &Client{
		t:   t,
		app: a,
		http: &http.Client{
			Jar: jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Get requests path, such as "/feed".
func (c *Client) Get(path string) *Response {
	c.t.Helper()

	req, err := http.NewRequest(http.MethodGet, c.app.URL+path, nil)
	if err != nil {
		c.t.Fatal(err)
	}

	return c.Do(req)
}

// PostForm posts url-encoded form values.
func (c *Client) PostForm(path string, form url.Values) *Response {
	c.t.Helper()

	req, err := http.NewRequest(http.MethodPost, c.app.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		c.t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.post(req)
}

// PostJSON posts v as JSON, the body the /controls/action endpoints accept.
func (c *Client) PostJSON(path string, v any) *Response {
	c.t.Helper()

	body, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, c.app.URL+path, bytes.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

	return c.post(req)
}

// LoginAs logs in by posting the login form, and fails the test unless the
// session can then open /feed.
func (c *Client) LoginAs(email, password string) {
	c.t.Helper()

	c.Get("/login").RequireStatus(http.StatusOK)

	resp := c.PostForm("/form/login", url.Values{"email": {email}, "password": {password}})

	if feed := c.Get("/feed"); feed.StatusCode != http.StatusOK {
		c.t.Fatalf("e2e: login as %s was rejected (status %d):\n%s", email, resp.StatusCode, resp.Body)
	}
}

// Do sends req, adding the CSRF token to anything but a GET.
func (c *Client) Do(req *http.Request) *Response {
	c.t.Helper()

	if req.Method != http.MethodGet && c.csrf != "" {
		req.Header.Set("X-CSRFToken", c.csrf)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("e2e: %s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}

	out := &Response{t: c.t, StatusCode: resp.StatusCode, Header: resp.Header, Body: string(body)}
	c.rememberCSRF(out)

	return out
}

// post makes sure a CSRF token is known before a mutating request: a fresh
// client loads the home page first, as a visitor's browser would have.
func (c *Client) post(req *http.Request) *Response {
	c.t.Helper()

	if c.csrf == "" {
		c.Get("/")
	}

	return c.Do(req)
}

func (c *Client) rememberCSRF(r *Response) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") {
		return
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(r.Body))
	if err != nil {
		return
	}

	raw, ok := doc.Find("body").Attr("hx-headers")
	if !ok {
		return
	}

	var headers map[string]string
	if err := json.Unmarshal([]byte(raw), &headers); err == nil && headers["X-CSRFToken"] != "" {
		c.csrf = headers["X-CSRFToken"]
	}
}

// Response is a fully read HTTP response.
type Response struct {
	t          testing.TB
	StatusCode int
	Header     http.Header
	Body       string
}

// RequireStatus fails the test unless the response has the given status.
func (r *Response) RequireStatus(code int) *Response {
	r.t.Helper()

	if r.StatusCode != code {
		r.t.Fatalf("e2e: status %d, want %d; body:\n%s", r.StatusCode, code, r.Body)
	}

	return r
}

// Doc parses the body as HTML for goquery CSS selectors.
func (r *Response) Doc() *goquery.Document {
	r.t.Helper()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(r.Body))
	if err != nil {
		r.t.Fatal(err)
	}

	return doc
}

// Location is the redirect target of a 3xx response.
func (r *Response) Location() string { return r.Header.Get("Location") }
