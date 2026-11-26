package reader_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/require"
)

const discoverRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>t</title></channel></rss>`

// page is an HTML page whose head holds head.
func page(head string) string {
	return "<!doctype html><html><head><title>site</title>" + head + "</head><body>hello</body></html>"
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	serve := func(path, contentType, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			_, _ = w.Write([]byte(body))
		})
	}

	serve("/feed.xml", "application/rss+xml", discoverRSS)
	serve("/relative", "text/html", page(`<link rel="alternate" type="application/rss+xml" href="/feed.xml">`))
	serve("/absolute", "text/html", page(`<link rel="alternate" type="application/atom+xml" href="https://elsewhere.example/atom">`))
	serve("/base/page", "text/html", page(`<base href="/blog/"><link rel="alternate" type="application/rss+xml" href="rss">`))
	serve("/first", "text/html", page(`<link rel="stylesheet" href="/s.css">`+
		`<link rel="alternate" type="application/json" href="/wp-json/wp/v2/pages/1">`+
		`<link rel="alternate" hreflang="de" href="/de/">`+
		`<link rel="Alternate Feed" type="application/rss+xml; charset=utf-8" href="/one.xml">`+
		`<link rel="alternate" type="application/atom+xml" href="/two.xml">`))
	serve("/json", "text/html", page(`<link rel="alternate" type="application/feed+json" href="/feed.json">`))
	serve("/none", "text/html", page(`<link rel="stylesheet" href="/s.css">`))
	serve("/mailto", "text/html", page(`<link rel="alternate" type="application/rss+xml" href="mailto:me@example.org">`))
	serve("/text", "text/plain", "just text")
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/sub/relative-here", http.StatusFound)
	})
	serve("/sub/relative-here", "text/html", page(`<link rel="alternate" type="application/rss+xml" href="feed.xml">`))
	mux.HandleFunc("/missing", http.NotFound)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	fetcher := reader.NewFetcher(&http.Client{})

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr error
	}{
		{"a feed is its own feed", "/feed.xml", srv.URL + "/feed.xml", nil},
		{"relative link", "/relative", srv.URL + "/feed.xml", nil},
		{"absolute link", "/absolute", "https://elsewhere.example/atom", nil},
		{"base href", "/base/page", srv.URL + "/blog/rss", nil},
		{"first feed link wins, other alternates skipped", "/first", srv.URL + "/one.xml", nil},
		{"json feed", "/json", srv.URL + "/feed.json", nil},
		{"resolved against the redirect target", "/moved", srv.URL + "/sub/feed.xml", nil},
		{"no feed link", "/none", "", reader.ErrNoFeedFound},
		{"non-http link", "/mailto", "", reader.ErrNoFeedFound},
		{"not html", "/text", "", reader.ErrNoFeedFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := fetcher.Discover(context.Background(), srv.URL+tt.path)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("error status", func(t *testing.T) {
		t.Parallel()

		_, err := fetcher.Discover(context.Background(), srv.URL+"/missing")
		require.ErrorContains(t, err, "404")
		require.NotErrorIs(t, err, reader.ErrNoFeedFound)
	})
}

func TestDiscover_TooLarge(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page(strings.Repeat("<meta name=x>", 1000))))
	}))
	t.Cleanup(srv.Close)

	fetcher := reader.NewFetcher(&http.Client{}, reader.WithLimits(reader.Limits{MaxFeedSize: 1024}))

	_, err := fetcher.Discover(context.Background(), srv.URL)
	require.ErrorIs(t, err, gofeed.ErrResponseTooLarge)
}

// DefaultFetcher refuses loopback addresses, which is where httptest serves,
// for every kind of download, unless private addresses are allowed. The
// check runs on the dialed address, so redirects and names that resolve to a
// private address are refused the same way.
func TestDefaultFetcher_PublicAddressesOnly(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(discoverRSS))
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	guarded := reader.DefaultFetcher(reader.Limits{})

	_, err := guarded.Fetch(ctx, srv.URL)
	require.ErrorIs(t, err, reader.ErrPrivateAddress)

	_, err = guarded.Discover(ctx, srv.URL)
	require.ErrorIs(t, err, reader.ErrPrivateAddress)

	_, err = guarded.FetchMedia(ctx, srv.URL)
	require.ErrorIs(t, err, reader.ErrPrivateAddress)

	// a name that resolves to a loopback address is refused just the same
	_, err = guarded.Discover(ctx, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1))
	require.ErrorIs(t, err, reader.ErrPrivateAddress)

	allowed := reader.DefaultFetcher(reader.Limits{AllowPrivateAddresses: true})
	got, err := allowed.Discover(ctx, srv.URL)
	require.NoError(t, err)
	require.Equal(t, srv.URL, got)
}

func TestIsPublicAddress(t *testing.T) {
	t.Parallel()

	public := []string{"93.184.215.14", "8.8.8.8", "2606:4700::1111", "::ffff:8.8.8.8"}
	private := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0",
		"100.64.0.1", "198.18.0.1", "192.0.2.1", "240.0.0.1", "255.255.255.255", "224.0.0.1",
		"::1", "::", "fe80::1", "fc00::1", "fd12::1", "ff02::1", "::ffff:127.0.0.1",
		"::ffff:169.254.169.254", "64:ff9b::7f00:1", "2002:7f00:1::", "2001:db8::1",
	}

	for _, s := range public {
		require.True(t, reader.IsPublicAddress(netip.MustParseAddr(s)), s)
	}

	for _, s := range private {
		require.False(t, reader.IsPublicAddress(netip.MustParseAddr(s)), s)
	}
}
