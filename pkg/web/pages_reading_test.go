package web

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// A share link's page shows the shared post and its author to whoever holds
// the link, titled by the post's subject.
func TestSharedPost(t *testing.T) {
	t.Parallel()

	author := &core.User{ID: "author", Username: "alice"}

	cases := []struct {
		name    string
		subject null.String
		want    string
	}{
		{"subject", null.StringFrom("Hello"), "Hello"},
		{"no subject", null.String{}, "No Subject"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			post := &core.Post{ID: "post", Subject: tc.subject}
			page := SharedPost(newTestContext(t, http.MethodGet, "/shared/x"), nil, &shares.Shared{Post: post, Author: author})

			require.Same(t, post, page.Post)
			require.Same(t, author, page.Author)
			require.Equal(t, tc.want, page.PostSubject)
			require.Equal(t, tc.want, page.Name)
		})
	}
}

// echoBackend "translates" every segment by prefixing it with "EN ".
type echoBackend struct{}

func (echoBackend) Name() string             { return "echo" }
func (echoBackend) DisplayName() string      { return "Echo" }
func (echoBackend) Limits() translate.Limits { return translate.Limits{Segments: 100, Chars: 50000} }

func (echoBackend) Translate(_ context.Context, req translate.Request) ([]string, error) {
	out := make([]string, len(req.Segments))
	for i, seg := range req.Segments {
		out[i] = "EN " + seg
	}

	return out, nil
}

// newTranslationView decides per post: the cached translation of a language
// the reader always translates is inline, an uncached one loads, another
// language gets a button, and a post its author keeps closed gets nothing.
func TestNewTranslationView(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	svc := translations.New(store, reading.New(store), translate.NewTranslator(echoBackend{}),
		translations.Limits{UserDailyChars: 1_000_000, SiteMonthlyChars: 1_000_000_000})

	author := testutil.Must(factory.User(ctx, db))(t)
	reader := testutil.Must(factory.User(ctx, db))(t)
	_, _, err := factory.Connect(ctx, db, author.ID, reader.ID)
	require.NoError(t, err)
	require.NoError(t, svc.SetLanguages(ctx, reader, []string{"de"}))

	post := func(lang string, opts ...factory.PostOpt) *postops.Post {
		opts = append([]factory.PostOpt{factory.Published(), factory.WithLanguage(lang)}, opts...)
		return &postops.Post{Post: testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t)}
	}

	cached := post("de", factory.AllowTranslation())
	_, err = svc.Translate(ctx, reader, core.TranslationSourceKindPost, cached.ID)
	require.NoError(t, err)

	uncached := post("de", factory.AllowTranslation())
	other := post("fr", factory.AllowTranslation())
	closed := post("de")

	view, err := newTranslationView(ctx, svc, reader, postSources(svc, reader, []*postops.Post{cached, uncached, other, closed}), nil)
	require.NoError(t, err)

	slot := func(v *TranslationView, p *postops.Post) map[string]any {
		return v.Slot("post", p.ID, "feed", "body", "xx")
	}

	require.Equal(t, true, slot(view, cached)["Translated"])
	require.Equal(t, "Echo", slot(view, cached)["Provider"])
	require.NotContains(t, slot(view, cached), "Load")
	require.Equal(t, true, slot(view, uncached)["Load"])
	require.NotContains(t, slot(view, uncached), "Translated")
	require.Equal(t, true, slot(view, other)["CanTranslate"])
	require.NotContains(t, slot(view, other), "Load")

	for _, p := range []*postops.Post{closed} {
		got := slot(view, p)
		require.NotContains(t, got, "CanTranslate")
		require.NotContains(t, got, "Load")
		require.NotContains(t, got, "Translated")
	}

	// anonymous, or translation off: no view, a plain original
	anon, err := newTranslationView(ctx, svc, nil, postSources(svc, nil, []*postops.Post{cached}), nil)
	require.NoError(t, err)
	require.Nil(t, anon)
	require.Equal(t, map[string]any{"Kind": "post", "ID": "p1", "View": "feed", "Body": "body", "Lang": "de"},
		anon.Slot("post", "p1", "feed", "body", "de"))

	off := translations.New(store, reading.New(store), nil, translations.Limits{})
	disabled, err := newTranslationView(ctx, off, reader, postSources(off, reader, []*postops.Post{cached}), nil)
	require.NoError(t, err)
	require.Nil(t, disabled)
}
