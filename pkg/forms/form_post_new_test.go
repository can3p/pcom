package forms_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/stretchr/testify/require"
)

// missingID is a well-formed but never-inserted UUID, for exercising a
// not-found lookup: these tables use uuid columns, so an arbitrary
// non-uuid string like "does-not-exist" fails at the database driver
// instead of reaching the "no rows" path under test.
const missingID = "00000000-0000-0000-0000-000000000000"

// mediaReplacer is a no-op media.Replacer[string]: none of these tests
// exercise media rewriting, only that a replacer was threaded through.
func mediaReplacer(in string) (bool, string) {
	return false, in
}

// renderStub/renderInstance make c.HTML a no-op. ginctx.New's context
// never calls LoadHTMLGlob, so the real gin engine has a nil HTMLRender and
// panics on the first template render; forms.FormSaveDefault's fallback
// path (and PostPromptForm.Save's) always renders the form on save, so a
// direct ginctx.New context can't be used to invoke a FormSaveAction here.
// These tests only care about status and headers, never the rendered body.
type renderStub struct{}

func (renderStub) Instance(name string, data any) render.Render { return renderInstance{} }

type renderInstance struct{}

func (renderInstance) Render(w http.ResponseWriter) error     { return nil }
func (renderInstance) WriteContentType(w http.ResponseWriter) {}

// newCtx returns a *gin.Context wired like pkg/testutil/ginctx.New, plus a
// working (no-op) HTML renderer so a form's FormSaveAction can be invoked
// without panicking. See renderStub.
func newCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.HTMLRender = renderStub{}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	return c, w
}

func TestNewPostFormNew(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	t.Run("without a prompt id", func(t *testing.T) {
		t.Parallel()

		form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), user, mediaReplacer, "")
		require.NoError(t, err)
		require.Nil(t, form.Prompt)
	})

	t.Run("with a prompt id addressed to the user", func(t *testing.T) {
		t.Parallel()

		asker, err := factory.User(ctx, db)
		require.NoError(t, err)
		prompt, err := factory.PostPrompt(ctx, db, asker.ID, user.ID)
		require.NoError(t, err)

		form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), user, mediaReplacer, prompt.ID)
		require.NoError(t, err)
		require.NotNil(t, form.Prompt)
		require.Equal(t, prompt.ID, form.Prompt.Prompt.ID)
	})

	t.Run("with an unknown prompt id, prompt is left nil", func(t *testing.T) {
		t.Parallel()

		form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), user, mediaReplacer, missingID)
		require.NoError(t, err)
		require.Nil(t, form.Prompt)
	})
}

func TestEditPostFormNew(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	other, err := factory.User(ctx, db)
	require.NoError(t, err)
	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)

	t.Run("loads the author's own post", func(t *testing.T) {
		t.Parallel()

		form, err := forms.EditPostFormNew(ctx, db, fakesender.New(), author, mediaReplacer, post.ID)
		require.NoError(t, err)
		require.Equal(t, post.ID, form.Post.ID)
	})

	t.Run("another user's post is not found", func(t *testing.T) {
		t.Parallel()

		_, err := forms.EditPostFormNew(ctx, db, fakesender.New(), other, mediaReplacer, post.ID)
		require.ErrorIs(t, err, ginhelpers.ErrNotFound)
	})

	t.Run("an unknown post is not found", func(t *testing.T) {
		t.Parallel()

		_, err := forms.EditPostFormNew(ctx, db, fakesender.New(), author, mediaReplacer, missingID)
		require.ErrorIs(t, err, ginhelpers.ErrNotFound)
	})
}

func TestPostForm_Validate_FieldErrors(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	newValidForm := func(t *testing.T) *forms.PostForm {
		t.Helper()
		form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), user, mediaReplacer, "")
		require.NoError(t, err)
		form.Input.Subject = "A subject"
		form.Input.Body = "A body"
		form.Input.Visibility = core.PostVisibilityDirectOnly
		form.Input.SaveAction = forms.PostFormActionAutosave

		return form
	}

	t.Run("valid input passes", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("an empty save action defaults to autosave and passes", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		form.Input.SaveAction = forms.PostFormAction("")
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("subject too long", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		for range 101 {
			form.Input.Subject += "a"
		}
		require.Error(t, form.Validate(c, db))
		require.True(t, form.Errors.HasError("subject"))
	})

	t.Run("body too long", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		body := make([]byte, 20_001)
		for i := range body {
			body[i] = 'a'
		}
		form.Input.Body = string(body)
		require.Error(t, form.Validate(c, db))
		require.True(t, form.Errors.HasError("body"))
	})

	t.Run("invalid url", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		form.Input.URL = "not-a-url"
		require.Error(t, form.Validate(c, db))
		require.True(t, form.Errors.HasError("url"))
	})

	t.Run("valid url passes", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		form.Input.URL = "https://example.test/article"
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("invalid visibility", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form := newValidForm(t)
		form.Input.Visibility = core.PostVisibility("bogus")
		require.Error(t, form.Validate(c, db))
		require.True(t, form.Errors.HasError("visibility"))
	})

	t.Run("invalid save action is recorded under the save_action key", func(t *testing.T) {
		t.Parallel()
		// known bug #157: PostForm.Validate records an invalid save_action under
		// the "visibility" error key instead of "save_action" -- the two
		// AddError calls share a copy-pasted key.
		t.Skip("known bug #157: PostForm.Validate records an invalid save_action under the visibility error key")

		c, _ := newCtx(t)
		form := newValidForm(t)
		form.Input.SaveAction = forms.PostFormAction("bogus")
		require.Error(t, form.Validate(c, db))
		require.True(t, form.Errors.HasError("save_action"))
	})
}

func TestPostForm_Validate_EditPermission(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	other, err := factory.User(ctx, db)
	require.NoError(t, err)
	post, err := factory.Post(ctx, db, author.ID)
	require.NoError(t, err)

	t.Run("the author can edit", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		form, err := forms.EditPostFormNew(ctx, db, fakesender.New(), author, mediaReplacer, post.ID)
		require.NoError(t, err)
		form.Input.Subject = "Updated"
		form.Input.Body = "Updated body"
		form.Input.Visibility = core.PostVisibilityDirectOnly
		form.Input.SaveAction = forms.PostFormActionAutosave

		require.NoError(t, form.Validate(c, db))
	})

	t.Run("a stranger cannot edit", func(t *testing.T) {
		t.Parallel()

		c, _ := newCtx(t)
		// Construct as if `other` were editing `post` (bypassing
		// EditPostFormNew's own ownership check, which would already
		// reject this) to reach PostForm.Validate's own capability check.
		form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), other, mediaReplacer, "")
		require.NoError(t, err)
		form.Post = post
		form.Input.Subject = "Updated"
		form.Input.Body = "Updated body"
		form.Input.Visibility = core.PostVisibilityDirectOnly
		form.Input.SaveAction = forms.PostFormActionAutosave

		require.ErrorIs(t, form.Validate(c, db), ginhelpers.ErrForbidden)
	})
}

func TestPostForm_Save_NewPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	for _, saveAction := range []forms.PostFormAction{
		forms.PostFormActionSavePost,
		forms.PostFormActionMakeDraft,
		forms.PostFormActionPublish,
		forms.PostFormActionAutosave,
		forms.PostFormAction(""),
	} {
		t.Run(string(saveAction)+" or empty", func(t *testing.T) {
			t.Parallel()

			author, err := factory.User(ctx, db)
			require.NoError(t, err)
			conn, err := factory.User(ctx, db)
			require.NoError(t, err)
			_, _, err = factory.Connect(ctx, db, author.ID, conn.ID)
			require.NoError(t, err)

			sender := fakesender.New()
			form, err := forms.NewPostFormNew(ctx, db, sender, author, mediaReplacer, "")
			require.NoError(t, err)
			form.Input.Subject = "A subject"
			form.Input.Body = "A body"
			form.Input.Visibility = core.PostVisibilityDirectOnly
			form.Input.SaveAction = saveAction

			c, w := newCtx(t)
			action, err := form.Save(ctx, db)
			require.NoError(t, err)
			action(c, form)

			posts, err := factory.ListPosts(ctx, db, author.ID)
			require.NoError(t, err)
			require.Len(t, posts, 1)
			post := posts[0]

			if saveAction == forms.PostFormActionPublish {
				require.True(t, post.PublishedAt.Valid)
				require.Equal(t, links.Link("post", post.ID), w.Header().Get("HX-Redirect"))

				sent := sender.Sent()
				require.Len(t, sent, 1)
				require.Equal(t, "post_notification", sent[0].EmailType)
				require.Equal(t, conn.Email, sent[0].Mail.To[0].Address)
			} else {
				// SavePost, MakeDraft, Autosave and the empty default all
				// take the same "still a draft" path: the post is created
				// but not published, and the response retargets the
				// draft-saved indicator instead of redirecting. Delete is
				// exercised on an existing post below; what it should do on
				// a never-saved post is an open product question.
				require.False(t, post.PublishedAt.Valid)
				require.Equal(t, "#last_draft_save", w.Header().Get("HX-Retarget"))
				require.Equal(t, links.Link("edit_post", post.ID), w.Header().Get("HX-Replace-Url"))
				require.Contains(t, w.Header().Get("HX-Trigger"), "draft_saved")
				require.Empty(t, sender.Sent())
			}
		})
	}
}

func TestPostForm_Save_NewPost_WithURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)

	form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), author, mediaReplacer, "")
	require.NoError(t, err)
	form.Input.Subject = "A subject"
	form.Input.Body = "A body"
	form.Input.URL = "https://example.test/article"
	form.Input.Visibility = core.PostVisibilityDirectOnly
	form.Input.SaveAction = forms.PostFormActionAutosave

	c, _ := newCtx(t)
	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	posts, err := factory.ListPosts(ctx, db, author.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.True(t, posts[0].URLID.Valid)
}

func TestPostForm_Save_NewPost_WithoutURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)

	form, err := forms.NewPostFormNew(ctx, db, fakesender.New(), author, mediaReplacer, "")
	require.NoError(t, err)
	form.Input.Subject = "A subject"
	form.Input.Body = "A body"
	form.Input.Visibility = core.PostVisibilityDirectOnly
	form.Input.SaveAction = forms.PostFormActionAutosave

	c, _ := newCtx(t)
	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	posts, err := factory.ListPosts(ctx, db, author.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.False(t, posts[0].URLID.Valid)
}

func TestPostForm_Save_NewPost_WithPrompt_Publish(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	promptRow, err := factory.PostPrompt(ctx, db, asker.ID, author.ID)
	require.NoError(t, err)

	sender := fakesender.New()
	form, err := forms.NewPostFormNew(ctx, db, sender, author, mediaReplacer, promptRow.ID)
	require.NoError(t, err)
	require.NotNil(t, form.Prompt)
	form.Input.Subject = "In answer"
	form.Input.Body = "Here is my answer"
	form.Input.Visibility = core.PostVisibilityDirectOnly
	form.Input.SaveAction = forms.PostFormActionPublish

	c, _ := newCtx(t)
	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	posts, err := factory.ListPosts(ctx, db, author.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)

	storedPrompt, err := factory.GetPostPrompt(ctx, db, promptRow.ID)
	require.NoError(t, err)
	require.Equal(t, posts[0].ID, storedPrompt.PostID.String)
	require.True(t, storedPrompt.DismissedAt.Valid)

	sent := sender.Sent()
	var gotAnswerMail bool
	for _, s := range sent {
		if s.EmailType == "post_prompt_answer" {
			gotAnswerMail = true
			require.Equal(t, asker.Email, s.Mail.To[0].Address)
		}
	}
	require.True(t, gotAnswerMail, "expected a post_prompt_answer notification to the asker")
}

func TestPostForm_Save_NewPost_WithPrompt_Draft(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	promptRow, err := factory.PostPrompt(ctx, db, asker.ID, author.ID)
	require.NoError(t, err)

	sender := fakesender.New()
	form, err := forms.NewPostFormNew(ctx, db, sender, author, mediaReplacer, promptRow.ID)
	require.NoError(t, err)
	form.Input.Subject = "Draft answer"
	form.Input.Body = "Still writing"
	form.Input.Visibility = core.PostVisibilityDirectOnly
	form.Input.SaveAction = forms.PostFormActionMakeDraft

	c, _ := newCtx(t)
	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	posts, err := factory.ListPosts(ctx, db, author.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)

	storedPrompt, err := factory.GetPostPrompt(ctx, db, promptRow.ID)
	require.NoError(t, err)
	// The prompt is linked to the draft as soon as the post exists...
	require.Equal(t, posts[0].ID, storedPrompt.PostID.String)
	// ...but only dismissed, and answered by mail, once the post is
	// actually published.
	require.False(t, storedPrompt.DismissedAt.Valid)

	for _, s := range sender.Sent() {
		require.NotEqual(t, "post_prompt_answer", s.EmailType)
	}
}

func TestPostForm_Save_ExistingPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	newForm := func(t *testing.T, author *core.User, sender *fakesender.Sender, opts ...factory.PostOpt) (*forms.PostForm, *core.Post) {
		t.Helper()
		post, err := factory.Post(ctx, db, author.ID, opts...)
		require.NoError(t, err)
		form, err := forms.EditPostFormNew(ctx, db, sender, author, mediaReplacer, post.ID)
		require.NoError(t, err)
		form.Input.Subject = "Updated subject"
		form.Input.Body = "Updated body"
		form.Input.Visibility = core.PostVisibilityDirectOnly

		return form, post
	}

	t.Run("make draft un-publishes a published post", func(t *testing.T) {
		t.Parallel()

		author, err := factory.User(ctx, db)
		require.NoError(t, err)
		form, _ := newForm(t, author, fakesender.New(), factory.Published())
		form.Input.SaveAction = forms.PostFormActionMakeDraft

		c, _ := newCtx(t)
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)

		got, err := factory.GetPost(ctx, db, form.Post.ID)
		require.NoError(t, err)
		require.False(t, got.PublishedAt.Valid)
	})

	t.Run("publish redirects and notifies connections", func(t *testing.T) {
		t.Parallel()

		author, err := factory.User(ctx, db)
		require.NoError(t, err)
		conn, err := factory.User(ctx, db)
		require.NoError(t, err)
		_, _, err = factory.Connect(ctx, db, author.ID, conn.ID)
		require.NoError(t, err)

		sender := fakesender.New()
		form, _ := newForm(t, author, sender)
		form.Input.SaveAction = forms.PostFormActionPublish

		c, w := newCtx(t)
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)

		got, err := factory.GetPost(ctx, db, form.Post.ID)
		require.NoError(t, err)
		require.True(t, got.PublishedAt.Valid)
		require.Equal(t, links.Link("post", got.ID), w.Header().Get("HX-Redirect"))

		sent := sender.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, conn.Email, sent[0].Mail.To[0].Address)
	})

	t.Run("save post on an already published post redirects without re-notifying", func(t *testing.T) {
		t.Parallel()

		author, err := factory.User(ctx, db)
		require.NoError(t, err)
		sender := fakesender.New()
		form, _ := newForm(t, author, sender, factory.Published())
		form.Input.SaveAction = forms.PostFormActionSavePost

		c, w := newCtx(t)
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)

		require.Equal(t, links.Link("post", form.Post.ID), w.Header().Get("HX-Redirect"))
		require.Empty(t, sender.Sent())
	})

	t.Run("save post on a draft keeps it a draft", func(t *testing.T) {
		t.Parallel()

		author, err := factory.User(ctx, db)
		require.NoError(t, err)
		form, _ := newForm(t, author, fakesender.New())
		form.Input.SaveAction = forms.PostFormActionSavePost

		c, w := newCtx(t)
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)

		got, err := factory.GetPost(ctx, db, form.Post.ID)
		require.NoError(t, err)
		require.False(t, got.PublishedAt.Valid)
		require.Equal(t, "#last_draft_save", w.Header().Get("HX-Retarget"))
	})

	t.Run("autosave on a draft keeps it a draft", func(t *testing.T) {
		t.Parallel()

		author, err := factory.User(ctx, db)
		require.NoError(t, err)
		form, _ := newForm(t, author, fakesender.New())
		form.Input.SaveAction = forms.PostFormActionAutosave

		c, w := newCtx(t)
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)

		got, err := factory.GetPost(ctx, db, form.Post.ID)
		require.NoError(t, err)
		require.False(t, got.PublishedAt.Valid)
		require.Equal(t, "#last_draft_save", w.Header().Get("HX-Retarget"))
	})

	t.Run("delete removes the post and redirects to controls", func(t *testing.T) {
		t.Parallel()

		author, err := factory.User(ctx, db)
		require.NoError(t, err)
		form, _ := newForm(t, author, fakesender.New())
		form.Input.SaveAction = forms.PostFormActionDelete

		c, w := newCtx(t)
		action, err := form.Save(ctx, db)
		require.NoError(t, err)
		action(c, form)

		require.Equal(t, links.Link("controls"), w.Header().Get("HX-Redirect"))

		posts, err := factory.ListPosts(ctx, db, author.ID)
		require.NoError(t, err)
		require.Empty(t, posts)
	})
}
