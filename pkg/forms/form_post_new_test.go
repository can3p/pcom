package forms_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// missingID is a well-formed UUID that is never inserted: the tables use
// uuid columns, so a non-uuid string would fail in the driver instead of
// reaching the not-found path.
const missingID = "00000000-0000-0000-0000-000000000000"

// postsService is the service the forms call, over db and a sender that
// records instead of sending.
func postsService(db *sqlx.DB, s repo.MailQueue) *posts.Service {
	return registry.New(db, registry.Deps{Sender: s}).Posts
}

// renderStub makes c.HTML a no-op: the forms' fallback save action renders
// the form, and a bare test engine has no HTML renderer.
type renderStub struct{}

func (renderStub) Instance(string, any) render.Render { return renderInstance{} }

type renderInstance struct{}

func (renderInstance) Render(http.ResponseWriter) error     { return nil }
func (renderInstance) WriteContentType(http.ResponseWriter) {}

// newCtx returns a POST context whose engine can render, so a form's save
// action can run.
func newCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.HTMLRender = renderStub{}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	return c, w
}

func fillPost(form *forms.PostForm, action forms.PostFormAction) *forms.PostForm {
	form.Input.Subject = "A subject"
	form.Input.Body = "A body"
	form.Input.Visibility = core.PostVisibilityDirectOnly
	form.Input.SaveAction = action

	return form
}

// savePost saves the form, runs the returned action and returns the response.
func savePost(t *testing.T, ctx context.Context, db *sqlx.DB, form *forms.PostForm) *httptest.ResponseRecorder {
	t.Helper()
	c, w := newCtx(t)
	action := testutil.Must(form.Save(ctx))(t)
	action(c, form)

	return w
}

func TestNewPostFormNew(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)
	asker := testutil.Must(factory.User(ctx, db))(t)
	prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, user.ID))(t)

	for _, tc := range []struct {
		name, promptID, wantPromptID string
	}{
		{"without a prompt id", "", ""},
		{"with a prompt addressed to the user", prompt.ID, prompt.ID},
		{"with an unknown prompt id", missingID, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			form := testutil.Must(forms.NewPostFormNew(ctx, postsService(db, fakesender.New()), user, tc.promptID))(t)
			if tc.wantPromptID == "" {
				require.Nil(t, form.Prompt)
			} else {
				require.Equal(t, tc.wantPromptID, form.Prompt.Prompt.ID)
			}
		})
	}
}

func TestEditPostFormNew(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

	form := testutil.Must(forms.EditPostFormNew(ctx, postsService(db, fakesender.New()), author, post.ID))(t)
	require.Equal(t, post.ID, form.Post.ID)

	_, err := forms.EditPostFormNew(ctx, postsService(db, fakesender.New()), other, post.ID)
	require.ErrorIs(t, err, service.ErrNotFound, "another user's post")
	_, err = forms.EditPostFormNew(ctx, postsService(db, fakesender.New()), author, missingID)
	require.ErrorIs(t, err, service.ErrNotFound, "an unknown post")
}

func TestPostForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, user.ID))(t)

	for _, tc := range []struct {
		name    string
		edit    func(*forms.PostForm)
		wantErr string // field with an error; "" means valid
		wantIs  error
	}{
		{name: "valid input"},
		{name: "empty save action defaults to autosave", edit: func(f *forms.PostForm) { f.Input.SaveAction = "" }},
		{name: "valid url", edit: func(f *forms.PostForm) { f.Input.URL = "https://example.test/article" }},
		{name: "subject too long", edit: func(f *forms.PostForm) { f.Input.Subject = strings.Repeat("a", 101) }, wantErr: "subject"},
		{name: "body too long", edit: func(f *forms.PostForm) { f.Input.Body = strings.Repeat("a", 20_001) }, wantErr: "body"},
		{name: "invalid url", edit: func(f *forms.PostForm) { f.Input.URL = "not-a-url" }, wantErr: "url"},
		{name: "invalid visibility", edit: func(f *forms.PostForm) { f.Input.Visibility = "bogus" }, wantErr: "visibility"},
		{
			name: "invalid save action", edit: func(f *forms.PostForm) { f.Input.SaveAction = "bogus" }, wantErr: "save_action",
		},
		{name: "the author editing their post", edit: func(f *forms.PostForm) { f.Post = post }},
		// Built without EditPostFormNew, whose ownership check would reject
		// it first, to reach Validate's own capability check.
		{name: "a stranger editing the post", edit: func(f *forms.PostForm) { f.User = other; f.Post = post }, wantIs: service.ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			form := fillPost(testutil.Must(forms.NewPostFormNew(ctx, postsService(db, fakesender.New()), user, ""))(t), forms.PostFormActionAutosave)
			if tc.edit != nil {
				tc.edit(form)
			}

			c, _ := newCtx(t)
			err := form.Validate(c)
			switch {
			case tc.wantIs != nil:
				require.ErrorIs(t, err, tc.wantIs)
			case tc.wantErr != "":
				require.Error(t, err)
				require.True(t, form.Errors.HasError(tc.wantErr))
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestPostForm_Save_NewPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	for _, action := range []forms.PostFormAction{
		forms.PostFormActionSavePost,
		forms.PostFormActionMakeDraft,
		forms.PostFormActionAutosave,
		"",
		forms.PostFormActionPublish,
	} {
		t.Run("action "+string(action), func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, db))(t)
			conn := testutil.Must(factory.User(ctx, db))(t)
			connect(t, ctx, db, author.ID, conn.ID)

			sender := fakesender.New()
			form := fillPost(testutil.Must(forms.NewPostFormNew(ctx, postsService(db, sender), author, ""))(t), action)
			w := savePost(t, ctx, db, form)

			posts := testutil.Must(factory.ListPosts(ctx, db, author.ID))(t)
			require.Len(t, posts, 1)
			post := posts[0]

			if action == forms.PostFormActionPublish {
				require.True(t, post.PublishedAt.Valid)
				require.Equal(t, links.Link("post", post.ID), w.Header().Get("HX-Redirect"))
				sent := sender.Sent()
				require.Len(t, sent, 1)
				require.Equal(t, "post_notification", sent[0].EmailType)
				require.Equal(t, conn.Email, sent[0].Mail.To[0].Address)

				return
			}

			// Every other action saves a draft and retargets the draft-saved
			// indicator instead of redirecting.
			require.False(t, post.PublishedAt.Valid)
			require.Equal(t, "#last_draft_save", w.Header().Get("HX-Retarget"))
			require.Equal(t, links.Link("edit_post", post.ID), w.Header().Get("HX-Replace-Url"))
			require.Contains(t, w.Header().Get("HX-Trigger"), "draft_saved")
			require.Empty(t, sender.Sent())
		})
	}

	t.Run("delete stores nothing", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, db))(t)
		form := fillPost(testutil.Must(forms.NewPostFormNew(ctx, postsService(db, fakesender.New()), author, ""))(t), forms.PostFormActionDelete)
		savePost(t, ctx, db, form)
		require.Empty(t, testutil.Must(factory.ListPosts(ctx, db, author.ID))(t))
	})

	for _, url := range []string{"", "https://example.test/article"} {
		t.Run("url "+url, func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, db))(t)
			form := fillPost(testutil.Must(forms.NewPostFormNew(ctx, postsService(db, fakesender.New()), author, ""))(t), forms.PostFormActionAutosave)
			form.Input.URL = url
			savePost(t, ctx, db, form)

			posts := testutil.Must(factory.ListPosts(ctx, db, author.ID))(t)
			require.Len(t, posts, 1)
			require.Equal(t, url != "", posts[0].URLID.Valid)
		})
	}

	// A prompt is linked to the post as soon as it exists, but dismissed and
	// answered by mail only once the post is published.
	for _, publish := range []bool{true, false} {
		t.Run(fmt.Sprintf("answering a prompt, publish=%v", publish), func(t *testing.T) {
			t.Parallel()

			asker := testutil.Must(factory.User(ctx, db))(t)
			author := testutil.Must(factory.User(ctx, db))(t)
			prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, author.ID))(t)

			action := forms.PostFormActionMakeDraft
			if publish {
				action = forms.PostFormActionPublish
			}
			sender := fakesender.New()
			form := fillPost(testutil.Must(forms.NewPostFormNew(ctx, postsService(db, sender), author, prompt.ID))(t), action)
			require.NotNil(t, form.Prompt)
			savePost(t, ctx, db, form)

			posts := testutil.Must(factory.ListPosts(ctx, db, author.ID))(t)
			require.Len(t, posts, 1)
			stored := testutil.Must(factory.GetPostPrompt(ctx, db, prompt.ID))(t)
			require.Equal(t, posts[0].ID, stored.PostID.String)
			require.Equal(t, publish, stored.DismissedAt.Valid)

			var answeredTo []string
			for _, s := range sender.Sent() {
				if s.EmailType == "post_prompt_answer" {
					answeredTo = append(answeredTo, s.Mail.To[0].Address)
				}
			}
			if publish {
				require.Equal(t, []string{asker.Email}, answeredTo)
			} else {
				require.Empty(t, answeredTo)
			}
		})
	}
}

func TestPostForm_Save_ExistingPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		published     bool
		action        forms.PostFormAction
		wantPublished bool
		wantDeleted   bool
		wantHeader    string // header name
		wantLink      func(postID string) string
		wantMail      bool
	}{
		{name: "make draft un-publishes", published: true, action: forms.PostFormActionMakeDraft},
		{
			name: "publish redirects and notifies", action: forms.PostFormActionPublish, wantPublished: true,
			wantHeader: "HX-Redirect", wantLink: func(id string) string { return links.Link("post", id) }, wantMail: true,
		},
		{
			name: "publish on a published post keeps its date without re-notifying", published: true, action: forms.PostFormActionPublish,
			wantPublished: true, wantHeader: "HX-Redirect", wantLink: func(id string) string { return links.Link("post", id) },
		},
		{
			name: "save on a published post redirects without re-notifying", published: true, action: forms.PostFormActionSavePost,
			wantPublished: true, wantHeader: "HX-Redirect", wantLink: func(id string) string { return links.Link("post", id) },
		},
		{name: "save on a draft keeps it a draft", action: forms.PostFormActionSavePost, wantHeader: "HX-Retarget", wantLink: func(string) string { return "#last_draft_save" }},
		{name: "autosave on a draft keeps it a draft", action: forms.PostFormActionAutosave, wantHeader: "HX-Retarget", wantLink: func(string) string { return "#last_draft_save" }},
		{
			name: "delete removes the post", action: forms.PostFormActionDelete, wantDeleted: true,
			wantHeader: "HX-Redirect", wantLink: func(string) string { return links.Link("controls") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, db))(t)
			conn := testutil.Must(factory.User(ctx, db))(t)
			connect(t, ctx, db, author.ID, conn.ID)
			var opts []factory.PostOpt
			if tc.published {
				opts = append(opts, factory.Published())
			}
			post := testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t)
			// read back, as the zone is lost on the way through Postgres
			before := testutil.Must(factory.GetPost(ctx, db, post.ID))(t)

			sender := fakesender.New()
			form := fillPost(testutil.Must(forms.EditPostFormNew(ctx, postsService(db, sender), author, post.ID))(t), tc.action)
			w := savePost(t, ctx, db, form)

			if tc.wantHeader != "" {
				require.Equal(t, tc.wantLink(post.ID), w.Header().Get(tc.wantHeader))
			}
			if tc.wantDeleted {
				require.Empty(t, testutil.Must(factory.ListPosts(ctx, db, author.ID))(t))
			} else {
				stored := testutil.Must(factory.GetPost(ctx, db, post.ID))(t)
				require.Equal(t, tc.wantPublished, stored.PublishedAt.Valid)
				if tc.published && tc.wantPublished {
					require.True(t, before.PublishedAt.Time.Equal(stored.PublishedAt.Time), "publish date must not move")
				}
			}
			if tc.wantMail {
				sent := sender.Sent()
				require.Len(t, sent, 1)
				require.Equal(t, conn.Email, sent[0].Mail.To[0].Address)
			} else {
				require.Empty(t, sender.Sent())
			}
		})
	}
}

// An unticked checkbox sends nothing, so the form only changes the stored
// choice when the editor showed the checkbox.
func TestPostForm_Save_AllowTranslation(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db))(t)

	form := fillPost(testutil.Must(forms.NewPostFormNew(ctx, postsService(db, nil), author, ""))(t), forms.PostFormActionSavePost)
	form.Input.TranslationShown = true
	form.Input.AllowTranslation = true
	savePost(t, ctx, db, form)

	stored := testutil.Must(factory.ListPosts(ctx, db, author.ID))(t)
	require.Len(t, stored, 1)
	require.True(t, stored[0].AllowTranslation)

	edit := fillPost(testutil.Must(forms.EditPostFormNew(ctx, postsService(db, nil), author, stored[0].ID))(t), forms.PostFormActionSavePost)
	savePost(t, ctx, db, edit)

	require.True(t, testutil.Must(factory.GetPost(ctx, db, stored[0].ID))(t).AllowTranslation, "an editor without the checkbox keeps the choice")

	edit.Input.TranslationShown = true
	savePost(t, ctx, db, edit)

	require.False(t, testutil.Must(factory.GetPost(ctx, db, stored[0].ID))(t).AllowTranslation, "an unticked box turns it off")
}
