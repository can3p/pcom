package translations_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

var errBackend = errors.New("backend down")

// fakeBackend prefixes every segment with "EN " and counts its calls.
type fakeBackend struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (*fakeBackend) Name() string             { return "fake" }
func (*fakeBackend) DisplayName() string      { return "Fake Translator" }
func (*fakeBackend) Limits() translate.Limits { return translate.Limits{Segments: 100, Chars: 50000} }

func (b *fakeBackend) Translate(_ context.Context, req translate.Request) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.calls++
	if b.err != nil {
		return nil, b.err
	}

	out := make([]string, len(req.Segments))
	for i, s := range req.Segments {
		out[i] = "EN " + s
	}

	return out, nil
}

func (b *fakeBackend) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.calls
}

var roomy = translations.Limits{UserDailyChars: 1_000_000, SiteMonthlyChars: 1_000_000_000}

func newService(db *sqlx.DB, limits translations.Limits, b *fakeBackend) *translations.Service {
	store := repo.New(db)
	return translations.New(store, reading.New(store), translate.NewTranslator(b, translate.WithAttempts(1)), limits)
}

func lang(code string, allow bool) factory.PostOpt {
	return func(p *core.Post) {
		p.Language = null.NewString(code, code != "")
		p.AllowTranslation = allow
		p.Body = "Привет, мир"
	}
}

func ruItem(i *core.RSSItem) {
	i.Language = null.StringFrom("ru")
	i.SanitizedDescription = "<p>Привет, <a href=\"https://example.com\">мир</a></p>"
}

// publicPost makes a reader and a public, published post in lang.
func publicPost(t *testing.T, db *sqlx.DB, opts ...factory.PostOpt) (*core.User, *core.Post) {
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db))(t)
	reader := testutil.Must(factory.User(ctx, db))(t)
	opts = append([]factory.PostOpt{factory.Published(), factory.Visibility(core.PostVisibilityPublic), lang("ru", true)}, opts...)

	return reader, testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t)
}

// makeStale translates the post for reader and then marks its row stale.
func makeStale(t *testing.T, store *repo.Store, svc *translations.Service, reader *core.User, postID string) {
	ctx := context.Background()
	_, err := svc.Translate(ctx, reader, core.TranslationSourceKindPost, postID)
	require.NoError(t, err)

	row := testutil.Must(store.Translation(ctx, core.TranslationSourceKindPost, postID, "en"))(t)
	row.SourceHash = "old"
	require.NoError(t, store.SaveTranslation(ctx, row))
}

type rowState int

const (
	rowNone rowState = iota
	rowFresh
	rowStale
)

type source func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string)

// subscribedItem makes a reader subscribed to a feed with a Russian item.
func subscribedItem(t *testing.T, db *sqlx.DB) (*core.User, *core.UserFeedSubscription, *core.RSSItem) {
	ctx := context.Background()
	reader := testutil.Must(factory.User(ctx, db))(t)
	feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
	item := testutil.Must(factory.RSSItem(ctx, db, feed.ID, ruItem))(t)
	sub := testutil.Must(factory.Subscription(ctx, db, reader.ID, feed.ID))(t)

	return reader, sub, item
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	post := core.TranslationSourceKindPost
	plain := func(opts ...factory.PostOpt) source {
		return func(t *testing.T, _ *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db, opts...)
			return reader, post, p.ID
		}
	}

	cases := []struct {
		name    string
		limits  translations.Limits
		prep    source
		wantErr error
		calls   int
		counted int // backend calls counted against the actor
		row     rowState
	}{
		{name: "anonymous", wantErr: service.ErrNeedsLogin, prep: func(t *testing.T, _ *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			_, p := publicPost(t, db)
			return nil, post, p.ID
		}},
		{name: "post the reader may not see", wantErr: service.ErrNotFound, prep: plain(factory.Visibility(core.PostVisibilityDirectOnly))},
		{name: "own draft", wantErr: service.ErrNotFound, prep: func(t *testing.T, _ *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			author := testutil.Must(factory.User(ctx, db))(t)
			p := testutil.Must(factory.Post(ctx, db, author.ID, lang("ru", true)))(t)
			return author, post, p.ID
		}},
		{name: "author doesn't allow translation", wantErr: translations.ErrNotTranslatable, prep: plain(lang("ru", false))},
		{name: "already English", wantErr: translations.ErrNotTranslatable, prep: plain(lang("en", true))},
		{name: "unknown language", wantErr: translations.ErrNotTranslatable, prep: plain(lang("", true))},
		{name: "translated", calls: 1, counted: 1, row: rowFresh, prep: plain()},
		{name: "cache hit", calls: 1, counted: 1, row: rowFresh, prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			_, err := svc.Translate(ctx, reader, post, p.ID)
			require.NoError(t, err)

			return reader, post, p.ID
		}},
		{name: "stale row is re-made", calls: 2, counted: 2, row: rowFresh, prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			makeStale(t, store, svc, reader, p.ID)

			return reader, post, p.ID
		}},
		{name: "re-translation queued", wantErr: translations.ErrUpdating, calls: 1, counted: 1, row: rowStale, prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			makeStale(t, store, svc, reader, p.ID)
			require.NoError(t, svc.RetranslateStale(ctx, store, p.ID))

			return reader, post, p.ID
		}},
		{name: "reader's daily limit", limits: translations.Limits{UserDailyChars: 5, SiteMonthlyChars: roomy.SiteMonthlyChars}, wantErr: translations.ErrLimit, prep: plain()},
		{name: "RSS item of a subscribed feed", calls: 1, counted: 1, row: rowFresh, prep: func(t *testing.T, _ *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, _, item := subscribedItem(t, db)
			return reader, core.TranslationSourceKindRSSItem, item.ID
		}},
		{name: "RSS item of a feed the reader doesn't follow", wantErr: service.ErrNotFound, prep: func(t *testing.T, _ *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			_, _, item := subscribedItem(t, db)
			return testutil.Must(factory.User(ctx, db))(t), core.TranslationSourceKindRSSItem, item.ID
		}},
		{name: "post ID as an RSS item", wantErr: service.ErrNotFound, prep: func(t *testing.T, _ *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			return reader, core.TranslationSourceKindRSSItem, p.ID
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			limits := tc.limits
			if limits == (translations.Limits{}) {
				limits = roomy
			}

			b := &fakeBackend{}
			svc := newService(db, limits, b)
			actor, kind, id := tc.prep(t, svc)

			res, err := svc.Translate(ctx, actor, kind, id)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.calls, b.count(), "backend calls")

			if tc.wantErr == nil {
				require.Equal(t, "Fake Translator", res.Provider)
				require.Equal(t, "ru", res.SourceLang)
				require.True(t, strings.HasPrefix(res.Subject, "EN "), res.Subject)
				require.Contains(t, res.Body, "EN Привет")
			}

			row := testutil.Must(store.Translation(ctx, kind, id, "en"))(t)

			switch tc.row {
			case rowNone:
				require.Nil(t, row)
			case rowFresh:
				require.NotNil(t, row)
				require.NotEqual(t, "old", row.SourceHash)
				require.Equal(t, "fake", row.Provider)
				require.Positive(t, row.Chars)
			case rowStale:
				require.Equal(t, "old", row.SourceHash)
			}

			if actor != nil {
				used := testutil.Must(store.TranslationCharsToday(ctx, actor.ID))(t)
				if tc.counted == 0 {
					require.Zero(t, used)
				} else {
					require.Equal(t, tc.counted*row.Chars, used)
				}
			}
		})
	}
}

// TestBudgets has the database to itself, so the site's total is known.
func TestBudgets(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	same := factory.WithSubject("Same subject")
	translateWith := func(limits translations.Limits, b *fakeBackend, reader *core.User) error {
		_, p := publicPost(t, db, same)
		_, err := newService(db, limits, b).Translate(ctx, reader, core.TranslationSourceKindPost, p.ID)

		return err
	}
	site := func() int { return testutil.Must(store.TranslationCharsThisMonth(ctx))(t) }

	first, p := publicPost(t, db, same)
	_, err := newService(db, roomy, &fakeBackend{}).Translate(ctx, first, core.TranslationSourceKindPost, p.ID)
	require.NoError(t, err)

	chars := site()
	require.Positive(t, chars)

	reader := testutil.Must(factory.User(ctx, db))(t)
	daily := translations.Limits{UserDailyChars: chars, SiteMonthlyChars: roomy.SiteMonthlyChars}
	require.NoError(t, translateWith(daily, &fakeBackend{}, reader), "the reader's budget is used up exactly")

	b := &fakeBackend{}
	require.ErrorIs(t, translateWith(daily, b, reader), translations.ErrLimit)
	require.Zero(t, b.count())

	other := testutil.Must(factory.User(ctx, db))(t)
	require.NoError(t, translateWith(translations.Limits{UserDailyChars: roomy.UserDailyChars, SiteMonthlyChars: 3 * chars}, &fakeBackend{}, other),
		"the site's budget is used up exactly")
	require.Equal(t, 3*chars, site())

	b = &fakeBackend{}
	require.ErrorIs(t, translateWith(translations.Limits{UserDailyChars: roomy.UserDailyChars, SiteMonthlyChars: 4*chars - 1}, b, other), translations.ErrLimit)
	require.Zero(t, b.count())

	b = &fakeBackend{err: errBackend}
	require.ErrorIs(t, translateWith(roomy, b, other), errBackend)
	require.Equal(t, 1, b.count())
	require.Equal(t, 3*chars, site(), "a backend error counts nothing")
}

func TestCached(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	post := core.TranslationSourceKindPost
	rssItem := core.TranslationSourceKindRSSItem

	cases := []struct {
		name string
		prep source
		want bool
	}{
		{name: "visible, current row", want: true, prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			testutil.Must(svc.Translate(ctx, reader, post, p.ID))(t)

			return reader, post, p.ID
		}},
		{name: "anonymous", prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			testutil.Must(svc.Translate(ctx, reader, post, p.ID))(t)

			return nil, post, p.ID
		}},
		{name: "stale row", prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			makeStale(t, store, svc, reader, p.ID)

			return reader, post, p.ID
		}},
		{name: "direct-only post the reader may not see", prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			_, p := publicPost(t, db, factory.Visibility(core.PostVisibilityDirectOnly))
			friend := testutil.Must(factory.User(ctx, db))(t)
			_, _, err := factory.Connect(ctx, db, p.UserID, friend.ID)
			require.NoError(t, err)
			testutil.Must(svc.Translate(ctx, friend, post, p.ID))(t)

			return testutil.Must(factory.User(ctx, db))(t), post, p.ID
		}},
		{name: "post unpublished after translation", prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, p := publicPost(t, db)
			testutil.Must(svc.Translate(ctx, reader, post, p.ID))(t)

			p.PublishedAt = null.Time{}
			require.NoError(t, store.UpdatePost(ctx, p))

			return reader, post, p.ID
		}},
		{name: "RSS item of a followed feed", want: true, prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, _, item := subscribedItem(t, db)
			testutil.Must(svc.Translate(ctx, reader, rssItem, item.ID))(t)

			return reader, rssItem, item.ID
		}},
		{name: "RSS item after the reader unsubscribed", prep: func(t *testing.T, svc *translations.Service) (*core.User, core.TranslationSourceKind, string) {
			reader, sub, item := subscribedItem(t, db)
			testutil.Must(svc.Translate(ctx, reader, rssItem, item.ID))(t)
			require.NoError(t, store.DeleteSubscription(ctx, reader.ID, sub.ID))

			return reader, rssItem, item.ID
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := &fakeBackend{}
			svc := newService(db, roomy, b)
			actor, kind, id := tc.prep(t, svc)
			calls := b.count()

			got := testutil.Must(svc.Cached(ctx, actor, kind, []string{id}))(t)
			require.Equal(t, calls, b.count(), "Cached never calls the backend")

			if !tc.want {
				require.Empty(t, got)
				return
			}

			require.Len(t, got, 1)
			require.Equal(t, "Fake Translator", got[id].Provider)
			require.Contains(t, got[id].Body, "EN Привет")
		})
	}
}

func TestForgetAndOff(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	svc := newService(db, roomy, &fakeBackend{})

	reader, p := publicPost(t, db)
	testutil.Must(svc.Translate(ctx, reader, core.TranslationSourceKindPost, p.ID))(t)
	require.True(t, svc.TranslatablePost(reader, p))
	require.False(t, svc.TranslatablePost(nil, p))

	require.NoError(t, svc.RetranslateStale(ctx, store, p.ID))
	require.Len(t, testutil.Must(store.TranslationJobsOfPost(ctx, p.ID))(t), 1)

	require.NoError(t, svc.Forget(ctx, store, p.ID))
	require.Nil(t, testutil.Must(store.Translation(ctx, core.TranslationSourceKindPost, p.ID, "en"))(t))
	require.Empty(t, testutil.Must(store.TranslationJobsOfPost(ctx, p.ID))(t))

	off := translations.New(store, reading.New(store), nil, roomy)
	require.False(t, off.Enabled())
	_, err := off.Translate(ctx, reader, core.TranslationSourceKindPost, p.ID)
	require.ErrorIs(t, err, translations.ErrNotTranslatable)
}

func TestSourceHashIncludesLanguage(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, translations.SourceHash("ru", "s", "b"), translations.SourceHash("uk", "s", "b"))
}

func TestLanguages(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := newService(db, roomy, &fakeBackend{})
	user := testutil.Must(factory.User(ctx, db))(t)

	require.NoError(t, svc.SetLanguages(ctx, user, []string{"ru", "DE", "ru"}))
	require.Equal(t, []string{"de", "ru"}, testutil.Must(svc.Languages(ctx, user))(t))

	var invalid *service.ValidationError
	require.ErrorAs(t, svc.SetLanguages(ctx, user, []string{"en"}), &invalid)
	require.ErrorAs(t, svc.SetLanguages(ctx, user, []string{"xx"}), &invalid)
	require.Equal(t, []string{"de", "ru"}, testutil.Must(svc.Languages(ctx, user))(t))

	require.ErrorIs(t, svc.SetLanguages(ctx, nil, []string{"ru"}), service.ErrNeedsLogin)
}

// TestWorker gives each case its own database: the worker takes every due job
// and the site's total is global.
func TestWorker(t *testing.T) {
	t.Parallel()

	update := func(f func(p *core.Post)) func(*testing.T, *repo.Store, *core.Post) {
		return func(t *testing.T, store *repo.Store, p *core.Post) {
			f(p)
			require.NoError(t, store.UpdatePost(context.Background(), p))
		}
	}

	cases := []struct {
		name      string
		edit      func(*testing.T, *repo.Store, *core.Post)
		fail      bool
		siteLimit bool // the site's budget holds only the reader's translation
		calls     int
		row       rowState
		attempts  int // of the job left; -1 when the job is gone
		counted   int // background calls counted against the site
	}{
		{name: "re-made", calls: 1, row: rowFresh, attempts: -1, counted: 1},
		{name: "backend fails", fail: true, calls: 1, row: rowStale, attempts: 1},
		{name: "post became a draft", edit: update(func(p *core.Post) { p.PublishedAt = null.Time{} }), row: rowStale, attempts: -1},
		{name: "author disallowed translation", edit: update(func(p *core.Post) { p.AllowTranslation = false }), row: rowStale, attempts: -1},
		{name: "site's budget spent: the job waits without an attempt", siteLimit: true, row: rowStale, attempts: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := testdb.New(t).DB
			store := repo.New(db)
			ctx := context.Background()

			reader, p := publicPost(t, db)
			makeStale(t, store, newService(db, roomy, &fakeBackend{}), reader, p.ID)
			chars := testutil.Must(store.Translation(ctx, core.TranslationSourceKindPost, p.ID, "en"))(t).Chars
			require.NoError(t, newService(db, roomy, &fakeBackend{}).RetranslateStale(ctx, store, p.ID))

			if tc.edit != nil {
				tc.edit(t, store, p)
			}

			limits := roomy
			if tc.siteLimit {
				limits.SiteMonthlyChars = chars
			}

			b := &fakeBackend{}
			if tc.fail {
				b.err = errBackend
			}

			svc := newService(db, limits, b)
			err := svc.RunJobs(ctx)
			if tc.fail {
				require.ErrorIs(t, err, errBackend)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.calls, b.count(), "backend calls")

			row := testutil.Must(store.Translation(ctx, core.TranslationSourceKindPost, p.ID, "en"))(t)
			if tc.row == rowFresh {
				require.NotEqual(t, "old", row.SourceHash)
			} else {
				require.Equal(t, "old", row.SourceHash)
			}

			jobs := testutil.Must(store.TranslationJobsOfPost(ctx, p.ID))(t)
			if tc.attempts < 0 {
				require.Empty(t, jobs)
			} else {
				require.Len(t, jobs, 1)
				require.Equal(t, tc.attempts, jobs[0].AttemptsNumber)
				require.Equal(t, core.TranslationJobStatusNew, jobs[0].Status)
				require.NoError(t, svc.RunJobs(ctx))
				require.Equal(t, tc.calls, b.count(), "a job waiting for its retry isn't taken")
			}

			require.Equal(t, (1+tc.counted)*chars, testutil.Must(store.TranslationCharsThisMonth(ctx))(t))
			require.Equal(t, chars, testutil.Must(store.TranslationCharsToday(ctx, reader.ID))(t), "the reader isn't charged")
		})
	}
}
