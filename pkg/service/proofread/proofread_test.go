package proofread_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/proofread/proofreadtest"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	proofreadsvc "github.com/can3p/pcom/pkg/service/proofread"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const msgModelFailed = "Claude did not answer. This attempt was not counted."

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

type env struct {
	store *repo.Store
	svc   *proofreadsvc.Service
	fake  *proofreadtest.Fake
	clock *clock
}

func newEnv(store *repo.Store, opts ...proofreadsvc.Option) *env {
	e := &env{store: store, fake: proofreadtest.New(), clock: &clock{now: time.Now()}}
	e.svc = proofreadsvc.New(store, posts.New(store, nil, nil), e.fake, append([]proofreadsvc.Option{proofreadsvc.WithClock(e.clock.Now)}, opts...)...)

	return e
}

func requireInvalid(t *testing.T, err error, msg string) {
	t.Helper()

	var verr *service.ValidationError
	require.ErrorAs(t, err, &verr)
	require.Equal(t, msg, verr.Message)
}

func requests(t *testing.T, store *repo.Store, userID string, status model.ProofreadStatus) int {
	t.Helper()

	c, err := store.ProofreadRequestsSince(context.Background(), repo.ProofreadRequestFilter{UserID: userID, Status: status})
	require.NoError(t, err)

	return c.Count
}

func TestAccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()

	cases := []struct {
		name string
		call func(e *env, me, other *model.User, mine, theirs *model.Post) error
		want error
		msg  string
	}{
		{"anonymous grammar", func(e *env, _, _ *model.User, _, _ *model.Post) error {
			_, err := e.svc.CheckGrammar(ctx, nil, proofreadsvc.GrammarRequest{Body: "x"})
			return err
		}, service.ErrNeedsLogin, ""},
		{"anonymous remaining", func(e *env, _, _ *model.User, _, _ *model.Post) error {
			_, err := e.svc.Remaining(ctx, nil)
			return err
		}, service.ErrNeedsLogin, ""},
		{"anonymous style", func(e *env, _, _ *model.User, mine, _ *model.Post) error {
			_, err := e.svc.ReviewStyle(ctx, nil, proofreadsvc.StyleRequest{PostID: mine.ID, Body: "x"})
			return err
		}, service.ErrNeedsLogin, ""},
		{"style of another user's post", func(e *env, me, _ *model.User, _, theirs *model.Post) error {
			_, err := e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: theirs.ID, Body: "x"})
			return err
		}, service.ErrForbidden, ""},
		{"state of another user's post", func(e *env, me, _ *model.User, _, theirs *model.Post) error {
			_, err := e.svc.StyleState(ctx, me, theirs.ID)
			return err
		}, service.ErrForbidden, ""},
		{"reset of another user's post", func(e *env, me, _ *model.User, _, theirs *model.Post) error {
			return e.svc.ResetStyle(ctx, me, theirs.ID)
		}, service.ErrForbidden, ""},
		{"note of another user's post", func(e *env, me, other *model.User, _, theirs *model.Post) error {
			e.fake.AnswerStyle(proofread.StyleOutput{Summary: "s", Notes: []proofread.Note{{Field: proofread.FieldNone, Text: "t"}}})
			r, err := e.svc.ReviewStyle(ctx, other, proofreadsvc.StyleRequest{PostID: theirs.ID, Body: "x"})
			if err != nil {
				return err
			}

			return e.svc.SetNoteStatus(ctx, me, r.Review.Notes[0].ID, model.StyleNoteStatusDone, "")
		}, service.ErrForbidden, ""},
		{"unknown post", func(e *env, me, _ *model.User, _, _ *model.Post) error {
			_, err := e.svc.StyleState(ctx, me, uuid.NewString())
			return err
		}, service.ErrNotFound, ""},
		{"malformed post id", func(e *env, me, _ *model.User, _, _ *model.Post) error {
			return e.svc.ResetStyle(ctx, me, "nope")
		}, service.ErrNotFound, ""},
		{"unknown note", func(e *env, me, _ *model.User, _, _ *model.Post) error {
			return e.svc.SetNoteStatus(ctx, me, uuid.NewString(), model.StyleNoteStatusDone, "")
		}, service.ErrNotFound, ""},
		{"draft without id", func(e *env, me, _ *model.User, _, _ *model.Post) error {
			_, err := e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{Body: "x"})
			return err
		}, nil, "Style review is available once the draft is saved."},
		{"subject too long, over the limit too", func(e *env, me, _ *model.User, _, _ *model.Post) error {
			if _, err := e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Body: "x"}); err != nil {
				return err
			}

			_, err := e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Subject: strings.Repeat("a", posts.DefaultPostSubjectMaxLength+1)})
			return err
		}, nil, "subject should be between 0 and 100 characters"},
		{"author note too long", func(e *env, me, _ *model.User, mine, _ *model.Post) error {
			_, err := e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: mine.ID, Body: "x", AuthorNote: strings.Repeat("a", proofreadsvc.DefaultNoteMaxLength+1)})
			return err
		}, nil, "note should be between 0 and 2000 characters"},
		{"grammar needs only the actor", func(e *env, me, _ *model.User, _, theirs *model.Post) error {
			_, err := e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{PostID: theirs.ID, Body: "x"})
			return err
		}, nil, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			me := testutil.Must(factory.User(ctx, db))(t)
			other := testutil.Must(factory.User(ctx, db))(t)
			mine := testutil.Must(factory.Post(ctx, db, me.ID))(t)
			theirs := testutil.Must(factory.Post(ctx, db, other.ID))(t)

			err := tc.call(newEnv(store, proofreadsvc.WithLimits(proofreadsvc.Limits{GrammarPerHour: 1})), me, other, mine, theirs)

			switch {
			case tc.want != nil:
				require.ErrorIs(t, err, tc.want)
			case tc.msg != "":
				requireInvalid(t, err, tc.msg)
			default:
				require.NoError(t, err)
			}
		})
	}

	t.Run("feature off", func(t *testing.T) {
		t.Parallel()

		me := testutil.Must(factory.User(ctx, db))(t)
		svc := proofreadsvc.New(store, posts.New(store, nil, nil), nil)
		require.False(t, svc.Enabled())

		_, err := svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Body: "x"})
		require.ErrorIs(t, err, service.ErrNotFound)
	})
}

func TestLimits(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	limits := proofreadsvc.Limits{GrammarPerHour: 2, StylePerHour: 2}

	kinds := []struct {
		name string
		noun string
		fail func(f *proofreadtest.Fake, err error)
		call func(e *env, me *model.User, post *model.Post) (proofreadsvc.Remaining, error)
		left func(r proofreadsvc.Remaining) int
	}{
		{"grammar", "grammar checks", (*proofreadtest.Fake).FailGrammarWith,
			func(e *env, me *model.User, _ *model.Post) (proofreadsvc.Remaining, error) {
				r, err := e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Body: "x"})
				if err != nil {
					return proofreadsvc.Remaining{}, err
				}

				return r.Remaining, nil
			}, func(r proofreadsvc.Remaining) int { return r.Grammar }},
		{"style", "style reviews", (*proofreadtest.Fake).FailStyleWith,
			func(e *env, me *model.User, post *model.Post) (proofreadsvc.Remaining, error) {
				r, err := e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: post.ID, Body: "x"})
				if err != nil {
					return proofreadsvc.Remaining{}, err
				}

				return r.Remaining, nil
			}, func(r proofreadsvc.Remaining) int { return r.Style }},
	}

	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()

			me := testutil.Must(factory.User(ctx, db))(t)
			post := testutil.Must(factory.Post(ctx, db, me.ID))(t)
			e := newEnv(store, proofreadsvc.WithLimits(limits))

			rem := testutil.Must(k.call(e, me, post))(t)
			require.Equal(t, 1, k.left(rem))

			e.clock.Advance(10 * time.Minute)
			rem = testutil.Must(k.call(e, me, post))(t)
			require.Equal(t, 0, k.left(rem), "at the limit")

			_, err := k.call(e, me, post)
			requireInvalid(t, err, "You have used all 2 "+k.noun+" for this hour. The next one is available in 50 minutes.")

			e.clock.Advance(51 * time.Minute)
			k.fail(e.fake, errors.New("overloaded"))
			_, err = k.call(e, me, post)
			requireInvalid(t, err, msgModelFailed)
			require.Equal(t, 1, requests(t, store, me.ID, model.ProofreadStatusFailed))

			rem = testutil.Must(e.svc.Remaining(ctx, me))(t)
			require.Equal(t, 1, k.left(rem), "the window slid and the failed call was not counted")

			k.fail(e.fake, nil)
			rem = testutil.Must(k.call(e, me, post))(t)
			require.Equal(t, 0, k.left(rem))
		})
	}
}

func TestSiteCap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cases := []struct {
		name    string
		cap     int
		refused bool
	}{
		{"at the cap", 2, true},
		{"no cap", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := testdb.New(t).DB // the cap counts the whole site
			e := newEnv(repo.New(db), proofreadsvc.WithLimits(proofreadsvc.Limits{SiteDailyRequests: tc.cap}))

			var err error
			for range 3 {
				me := testutil.Must(factory.User(ctx, db))(t)
				_, err = e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Body: "x"})
			}

			if !tc.refused {
				require.NoError(t, err)
				return
			}

			requireInvalid(t, err, "Grammar checks and style reviews are not available right now.")

			e.clock.Advance(24*time.Hour + time.Minute)
			me := testutil.Must(factory.User(ctx, db))(t)
			_, err = e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Body: "x"})
			require.NoError(t, err)
		})
	}
}

func TestCheckGrammar_KeepsWhatTheTextHolds(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	e := newEnv(repo.New(db))
	me := testutil.Must(factory.User(ctx, db))(t)

	e.fake.AnswerGrammar(proofread.GrammarOutput{Suggestions: []proofread.Suggestion{
		{Field: proofread.FieldBody, Kind: proofread.GrammarKindSpelling, Quote: "wrold", Before: "Bye ", Replacement: "world", Explanation: "typo"},
		{Field: proofread.FieldBody, Kind: proofread.GrammarKindSpelling, Quote: "made up", Replacement: "real"},
	}})

	res := testutil.Must(e.svc.CheckGrammar(ctx, me, proofreadsvc.GrammarRequest{Subject: "S", Body: "Hi wrold. Bye wrold."}))(t)
	require.Equal(t, []proofreadsvc.Suggestion{{
		Field: proofread.FieldBody, Kind: proofread.GrammarKindSpelling, Quote: "wrold", Before: "Bye ",
		Occurrence: 0, Replacement: "world", Explanation: "typo",
	}}, res.Suggestions)
	require.Equal(t, proofreadsvc.DefaultGrammarModel, e.fake.GrammarCalls()[0].Model.Name)
	require.Equal(t, 1, requests(t, e.store, me.ID, model.ProofreadStatusOK))
}

func TestReviewStyle_FollowsUpThePreviousReview(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	e := newEnv(store)
	me := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, me.ID))(t)
	body := "One. Two. Three. Four. Two."

	e.fake.AnswerStyle(proofread.StyleOutput{Summary: "Overall fine", Notes: []proofread.Note{
		{Field: proofread.FieldBody, Kind: proofread.NoteKindPacing, Quote: "One.", Text: "slow"},
		{Field: proofread.FieldBody, Kind: proofread.NoteKindLogic, Quote: "Two.", Before: "Four. ", Text: "why"},
		{Field: proofread.FieldBody, Kind: proofread.NoteKindClarity, Quote: "Three.", Text: "vague"},
		{Field: proofread.FieldBody, Kind: proofread.NoteKindOther, Quote: "Four.", Text: "cut"},
		{Field: proofread.FieldBody, Kind: proofread.NoteKindOther, Quote: "Five.", Text: "made up"},
	}})

	first := testutil.Must(e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: post.ID, Body: body, AuthorNote: "first"}))(t).Review
	require.Equal(t, 1, first.Number)
	require.Len(t, first.Notes, 4, "the made-up quote is dropped")
	require.Equal(t, "Four. ", first.Notes[1].Before)
	require.Equal(t, 0, first.Notes[1].Occurrence)
	require.Nil(t, e.fake.StyleCalls()[0].Previous)

	continued, rewritten, done, kept := first.Notes[0], first.Notes[1], first.Notes[2], first.Notes[3]
	require.NoError(t, e.svc.SetNoteStatus(ctx, me, done.ID, model.StyleNoteStatusDone, ""))
	require.NoError(t, e.svc.SetNoteStatus(ctx, me, kept.ID, model.StyleNoteStatusKept, " on purpose "))

	e.fake.AnswerStyle(proofread.StyleOutput{Summary: "Better", Notes: []proofread.Note{
		{Field: proofread.FieldBody, Kind: proofread.NoteKindPacing, Quote: "One.", Text: "still slow", Continues: continued.ID},
		{Field: proofread.FieldNone, Kind: proofread.NoteKindOther, Text: "general", Continues: "invented"},
	}})

	second := testutil.Must(e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: post.ID, Body: body, AuthorNote: "second"}))(t).Review
	require.Equal(t, 2, second.Number)
	require.Equal(t, 1, second.Notes[0].ContinuesNumber, "still open from review 1")
	require.Equal(t, 0, second.Notes[1].ContinuesNumber, "an unknown note is not continued")

	in := e.fake.StyleCalls()[1]
	require.Equal(t, "second", in.AuthorNote)
	require.Equal(t, "first", in.Previous.AuthorNote)
	require.Equal(t, "on purpose", in.Previous.Notes[3].KeepReason)
	require.Equal(t, proofread.NoteStatusKept, in.Previous.Notes[3].Status)

	reviews := testutil.Must(store.StyleReviews(ctx, post.ID))(t)
	statuses := map[string]model.StyleNoteStatus{}
	for _, n := range reviews[1].Notes {
		statuses[n.ID] = n.Status
	}

	require.Equal(t, map[string]model.StyleNoteStatus{
		continued.ID: model.StyleNoteStatusCarried,
		rewritten.ID: model.StyleNoteStatusDone,
		done.ID:      model.StyleNoteStatusDone,
		kept.ID:      model.StyleNoteStatusKept,
	}, statuses)

	e.fake.AnswerStyle(proofread.StyleOutput{Summary: "Third", Notes: []proofread.Note{
		{Field: proofread.FieldBody, Kind: proofread.NoteKindPacing, Quote: "One.", Text: "slow again", Continues: second.Notes[0].ID},
	}})
	third := testutil.Must(e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: post.ID, Body: body}))(t).Review
	require.Equal(t, 1, third.Notes[0].ContinuesNumber, "open since the review that raised it")

	state := testutil.Must(e.svc.StyleState(ctx, me, post.ID))(t)
	require.Equal(t, third.Notes, state.Review.Notes)
	require.Equal(t, []int{2, 1}, []int{state.Earlier[0].Number, state.Earlier[1].Number})
}

func TestSetNoteStatus(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()

	review := func(t *testing.T, e *env, me *model.User, post *model.Post) *proofreadsvc.Review {
		e.fake.AnswerStyle(proofread.StyleOutput{Summary: "s", Notes: []proofread.Note{{Field: proofread.FieldNone, Text: "t"}}})
		return testutil.Must(e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: post.ID, Body: "x"}))(t).Review
	}

	cases := []struct {
		name       string
		reviews    int
		before     model.StyleNoteStatus
		status     model.StyleNoteStatus
		reason     string
		msg        string
		wantReason string
	}{
		{"kept with a reason", 1, "", model.StyleNoteStatusKept, "my voice", "", "my voice"},
		{"done drops the reason", 1, "", model.StyleNoteStatusDone, "ignored", "", ""},
		{"open again", 1, model.StyleNoteStatusKept, model.StyleNoteStatusOpen, "", "", ""},
		{"reason too long", 1, "", model.StyleNoteStatusKept, strings.Repeat("a", proofreadsvc.DefaultNoteMaxLength+1), "reason should be between 0 and 2000 characters", ""},
		{"carried is the service's", 1, "", model.StyleNoteStatusCarried, "", "A note can only be marked done, kept as is, or open.", ""},
		{"an earlier review's note", 2, "", model.StyleNoteStatusDone, "", "Only the notes of the latest review can be changed.", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			me := testutil.Must(factory.User(ctx, db))(t)
			post := testutil.Must(factory.Post(ctx, db, me.ID))(t)
			e := newEnv(store)

			note := review(t, e, me, post).Notes[0]
			for range tc.reviews - 1 {
				review(t, e, me, post)
			}

			if tc.before != "" {
				require.NoError(t, e.svc.SetNoteStatus(ctx, me, note.ID, tc.before, "a reason"))
			}

			err := e.svc.SetNoteStatus(ctx, me, note.ID, tc.status, tc.reason)
			if tc.msg != "" {
				requireInvalid(t, err, tc.msg)
				return
			}

			require.NoError(t, err)

			got := testutil.Must(e.svc.StyleState(ctx, me, post.ID))(t).Review.Notes[0]
			require.Equal(t, tc.status, got.Status)
			require.Equal(t, tc.wantReason, got.KeepReason)
		})
	}
}

func TestStyleReviews_GoAway(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()

	cases := []struct {
		name   string
		remove func(e *env, me *model.User, post *model.Post) error
	}{
		{"start fresh", func(e *env, me *model.User, post *model.Post) error {
			return e.svc.ResetStyle(ctx, me, post.ID)
		}},
		{"post deleted", func(_ *env, me *model.User, post *model.Post) error {
			return posts.New(store, nil, nil).Delete(ctx, me, post.ID)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			me := testutil.Must(factory.User(ctx, db))(t)
			post := testutil.Must(factory.Post(ctx, db, me.ID))(t)
			e := newEnv(store)

			e.fake.AnswerStyle(proofread.StyleOutput{Summary: "s", Notes: []proofread.Note{{Field: proofread.FieldNone, Text: "t"}}})
			_, err := e.svc.ReviewStyle(ctx, me, proofreadsvc.StyleRequest{PostID: post.ID, Body: "x"})
			require.NoError(t, err)
			require.Len(t, testutil.Must(store.StyleReviews(ctx, post.ID))(t), 1)

			require.NoError(t, tc.remove(e, me, post))
			require.Empty(t, testutil.Must(store.StyleReviews(ctx, post.ID))(t))
		})
	}
}

func TestReviewStyle_StoresAllOrNothing(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	e := newEnv(store)
	me := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, me.ID))(t)
	req := proofreadsvc.StyleRequest{PostID: post.ID, Body: "One. Two."}

	e.fake.AnswerStyle(proofread.StyleOutput{Summary: "first", Notes: []proofread.Note{
		{Field: proofread.FieldBody, Kind: proofread.NoteKindPacing, Quote: "One.", Text: "slow"},
	}})
	first := testutil.Must(e.svc.ReviewStyle(ctx, me, req))(t).Review

	// Postgres refuses a NUL byte in text, so the second review's note fails
	// to insert after its request row and the review are written.
	e.fake.AnswerStyle(proofread.StyleOutput{Summary: "second", Notes: []proofread.Note{
		{Field: proofread.FieldNone, Kind: proofread.NoteKindOther, Text: "bad\x00"},
	}})
	_, err := e.svc.ReviewStyle(ctx, me, req)
	require.Error(t, err)

	reviews := testutil.Must(store.StyleReviews(ctx, post.ID))(t)
	require.Len(t, reviews, 1)
	require.Equal(t, first.ID, reviews[0].ID)
	require.Len(t, reviews[0].Notes, 1)
	require.Equal(t, model.StyleNoteStatusOpen, reviews[0].Notes[0].Status)
	require.Equal(t, 1, requests(t, store, me.ID, model.ProofreadStatusOK))
}
