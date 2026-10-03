package posts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// fakeTranslations records what the save transaction asked of it.
type fakeTranslations struct{ calls []string }

func (f *fakeTranslations) Enabled() bool { return true }

func (f *fakeTranslations) RetranslateStale(_ context.Context, _ *repo.Store, id string) error {
	f.calls = append(f.calls, "retranslate "+id)
	return nil
}

func (f *fakeTranslations) Forget(_ context.Context, _ *repo.Store, id string) error {
	f.calls = append(f.calls, "forget "+id)
	return nil
}

const german = "Der schnelle braune Fuchs springt über den faulen Hund, während das Wetter schön bleibt und alle nach Hause gehen."

const english = "The quick brown fox jumps over the lazy dog while the weather stays fine and everybody goes home."

func TestSave_Translation(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db))(t)

	yes, no := true, false

	newSvc := func() (*posts.Service, *fakeTranslations) {
		fake := &fakeTranslations{}
		return posts.New(repo.New(db), nil, nil, posts.WithTranslations(fake)), fake
	}

	save := func(t *testing.T, s *posts.Service, in posts.SaveInput) *core.Post {
		t.Helper()

		in.Visibility = core.PostVisibilityPublic

		return testutil.Must(s.Save(ctx, author, in))(t).Post
	}

	t.Run("language is detected on publish and edit, null when unsure", func(t *testing.T) {
		t.Parallel()

		s, _ := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish})
		require.Equal(t, null.StringFrom("en"), testutil.Must(factory.GetPost(ctx, db, p.ID))(t).Language)

		p = save(t, s, posts.SaveInput{PostID: p.ID, Body: "ok", Action: posts.ActionSavePost})
		stored := testutil.Must(factory.GetPost(ctx, db, p.ID))(t)
		require.False(t, stored.Language.Valid)
	})

	t.Run("toggle is stored and nil keeps it", func(t *testing.T) {
		t.Parallel()

		s, _ := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish})
		require.False(t, p.AllowTranslation)

		p = save(t, s, posts.SaveInput{PostID: p.ID, Body: english, Action: posts.ActionSavePost, AllowTranslation: &yes})
		require.True(t, p.AllowTranslation)

		p = save(t, s, posts.SaveInput{PostID: p.ID, Body: english, Action: posts.ActionSavePost})
		require.True(t, testutil.Must(factory.GetPost(ctx, db, p.ID))(t).AllowTranslation)
	})

	t.Run("changed text of a published post retranslates, same text does not", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish, AllowTranslation: &yes})
		require.Empty(t, fake.calls, "a new post has no translations")

		save(t, s, posts.SaveInput{PostID: p.ID, Body: english, Action: posts.ActionSavePost})
		require.Empty(t, fake.calls)

		save(t, s, posts.SaveInput{PostID: p.ID, Body: english + " Edited.", Action: posts.ActionSavePost})
		require.Equal(t, []string{"retranslate " + p.ID}, fake.calls)
	})

	t.Run("turning the toggle off forgets", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish, AllowTranslation: &yes})
		save(t, s, posts.SaveInput{PostID: p.ID, Body: english, Action: posts.ActionSavePost, AllowTranslation: &no})
		require.Equal(t, []string{"forget " + p.ID}, fake.calls)
	})

	t.Run("a subject-only change retranslates once", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Subject: "one", Body: english, Action: posts.ActionPublish, AllowTranslation: &yes})
		save(t, s, posts.SaveInput{PostID: p.ID, Subject: "two", Body: english, Action: posts.ActionSavePost})
		require.Equal(t, []string{"retranslate " + p.ID}, fake.calls)
	})

	t.Run("toggle off with a body change only forgets", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish, AllowTranslation: &yes})
		save(t, s, posts.SaveInput{PostID: p.ID, Body: english + " Edited.", Action: posts.ActionSavePost, AllowTranslation: &no})
		require.Equal(t, []string{"forget " + p.ID}, fake.calls)
	})

	t.Run("a published post saved as a draft with the toggle off forgets", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish, AllowTranslation: &yes})
		save(t, s, posts.SaveInput{PostID: p.ID, Body: english, Action: posts.ActionMakeDraft, AllowTranslation: &no})
		require.Equal(t, []string{"forget " + p.ID}, fake.calls)
	})

	t.Run("unpublish, edit, republish retranslates", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionPublish, AllowTranslation: &yes})
		save(t, s, posts.SaveInput{PostID: p.ID, Body: english, Action: posts.ActionMakeDraft})
		require.Empty(t, fake.calls)

		save(t, s, posts.SaveInput{PostID: p.ID, Body: english + " Edited.", Action: posts.ActionPublish})
		require.Equal(t, []string{"retranslate " + p.ID}, fake.calls)
	})

	t.Run("a draft edit never retranslates", func(t *testing.T) {
		t.Parallel()

		s, fake := newSvc()
		p := save(t, s, posts.SaveInput{Body: english, Action: posts.ActionMakeDraft, AllowTranslation: &yes})
		save(t, s, posts.SaveInput{PostID: p.ID, Body: english + " Edited.", Action: posts.ActionMakeDraft})
		require.Empty(t, fake.calls)
	})

	t.Run("german is detected and stored", func(t *testing.T) {
		t.Parallel()

		s, _ := newSvc()
		p := save(t, s, posts.SaveInput{Body: german, Action: posts.ActionPublish})
		require.Equal(t, null.StringFrom("de"), testutil.Must(factory.GetPost(ctx, db, p.ID))(t).Language)
	})
}
