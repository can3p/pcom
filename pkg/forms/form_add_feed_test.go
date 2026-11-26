package forms_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestAddFeedForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := feeds.New(repo.New(db), nil)
	ctx := context.Background()

	tests := []struct {
		name         string
		url          string
		wantErrField string
	}{
		{"empty url", "", "url"},
		{"invalid url", "not-a-valid-url", "url"},
		{"no protocol", "example.com/feed.xml", "url"},
		{"valid http url", "http://example.com/feed.xml", ""},
		{"valid https url", "https://example.com/feed.xml", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

			form := forms.NewAddFeedForm(svc, user)
			form.Input.URL = tt.url

			err := form.Validate(c)
			if tt.wantErrField == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, form.Errors.HasError(tt.wantErrField))
		})
	}
}

func TestAddFeedForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := feeds.New(repo.New(db), nil, feeds.WithFetchLimits(reader.Limits{AllowPrivateAddresses: true}))
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title></channel></rss>`))
	})
	mux.HandleFunc("/site", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><link rel="alternate" type="application/rss+xml" href="/feed.xml"></head></html>`))
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>no feed</title></head></html>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		url        string
		wantErr    bool
		wantURL    string
		wantRefuse string
	}{
		// Save doesn't re-run Validate, so a whitespace-only URL reaches
		// feeds.Subscribe's own error path.
		{"blank url fails", "   ", true, "", ""},
		{"trims url", "  " + srv.URL + "/feed.xml  ", false, srv.URL + "/feed.xml", ""},
		{"website subscribes to its feed", srv.URL + "/site", false, srv.URL + "/feed.xml", ""},
		{"website without a feed shows under the field", srv.URL + "/plain", false, "", "No RSS or Atom feed found at this address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

			form := forms.NewAddFeedForm(svc, user)
			form.Input.URL = tt.url

			action, err := form.Save(c)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, action)

			if tt.wantRefuse != "" {
				require.False(t, form.FormSaved)
				require.Equal(t, tt.wantRefuse, form.Errors["url"])
				require.Empty(t, testutil.Must(svc.Subscriptions(ctx, user))(t))

				return
			}

			require.True(t, form.FormSaved)
			require.Empty(t, form.Input.URL, "the section comes back with an empty form")

			subs := testutil.Must(svc.Subscriptions(ctx, user))(t)
			require.Len(t, subs, 1)
			require.Equal(t, tt.wantURL, subs[0].URL)
		})
	}
}
