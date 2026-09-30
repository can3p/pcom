package forms_test

import (
	"context"
	"net/http"
	"testing"

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

			err := form.Validate(c, db)
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
	svc := feeds.New(repo.New(db), nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		url     string
		wantErr bool
		wantURL string
	}{
		// Save doesn't re-run Validate, so a whitespace-only URL reaches
		// feeds.Subscribe's own error path.
		{"blank url fails", "   ", true, ""},
		{"trims url", "  https://example.com/feed.xml  ", false, "https://example.com/feed.xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

			form := forms.NewAddFeedForm(svc, user)
			form.Input.URL = tt.url

			action, err := form.Save(c, db)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, action)

			subs := testutil.Must(svc.Subscriptions(ctx, user))(t)
			require.Len(t, subs, 1)
			require.Equal(t, tt.wantURL, subs[0].URL)
		})
	}
}
