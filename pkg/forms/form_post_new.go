package forms

import (
	"context"
	"errors"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
)

type PostFormInput struct {
	Subject    string              `form:"subject"`
	URL        string              `form:"url"`
	Body       string              `form:"body"`
	Visibility core.PostVisibility `form:"visibility"`
	SaveAction PostFormAction      `form:"save_action"`
}

type PostForm struct {
	*forms.FormBase[PostFormInput]
	User   *core.User
	Posts  *posts.Service
	Post   *core.Post
	Prompt *postops.PostPrompt
}

type PostFormAction string

func (p PostFormAction) String() string {
	return string(p)
}

const (
	PostFormActionSavePost  PostFormAction = "save_post"
	PostFormActionMakeDraft PostFormAction = "make_draft"
	PostFormActionPublish   PostFormAction = "publish"
	PostFormActionDelete    PostFormAction = "delete"
	PostFormActionAutosave  PostFormAction = "autosave"
)

func NewPostFormNew(ctx context.Context, svc *posts.Service, u *core.User, promptID string) (*PostForm, error) {
	var prompt *postops.PostPrompt

	if promptID != "" {
		var err error

		prompt, err = svc.PromptFor(ctx, u, promptID)
		if err != nil {
			return nil, err
		}
	}

	form := &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]any{
				"User":   u,
				"Prompt": prompt,
			},
		},
		User:   u,
		Posts:  svc,
		Prompt: prompt,
	}

	return form, nil
}

func EditPostFormNew(ctx context.Context, svc *posts.Service, u *core.User, postID string) (*PostForm, error) {
	view, err := svc.ForEdit(ctx, u, postID)
	if errors.Is(err, service.ErrForbidden) {
		// somebody else's post is none of the actor's business
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	post := view.Post

	form := &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]any{
				"User":          u,
				"PostID":        post.ID,
				"IsPublished":   post.PublishedAt.Valid,
				"LastUpdatedAt": post.UpdatedAt.Time,
				"Prompt":        view.Prompt,
			},
		},
		User:   u,
		Posts:  svc,
		Post:   post,
		Prompt: view.Prompt,
	}

	return form, nil
}

// saveInput is what the form's input asks the service to save.
func (f *PostForm) saveInput() posts.SaveInput {
	in := posts.SaveInput{
		Subject:    f.Input.Subject,
		URL:        f.Input.URL,
		Body:       f.Input.Body,
		Visibility: f.Input.Visibility,
		Action:     posts.Action(f.Input.SaveAction),
	}

	switch {
	case f.Post != nil:
		in.PostID = f.Post.ID
	case f.Prompt != nil:
		in.PromptID = f.Prompt.Prompt.ID
	}

	return in
}

func (f *PostForm) Validate(c *gin.Context) error {
	for field, message := range f.Posts.ValidateSave(f.saveInput()) {
		f.AddError(field, message)
	}

	// this sounds like too much, but this way
	// we put the permission logic into a single place
	// and do not rely on adhoc queries
	if f.Post != nil {
		if err := f.Posts.CheckEdit(c, f.User, f.Post); err != nil {
			return err
		}
	}

	return f.Errors.PassedValidation()
}

func (f *PostForm) Save(c context.Context) (forms.FormSaveAction, error) {
	saved, err := f.Posts.Save(c, f.User, f.saveInput())
	if err != nil {
		return nil, err
	}

	if saved.Deleted {
		return forms.FormSaveRedirect(links.Link("controls")), nil
	}

	post := saved.Post
	saveAction := f.Input.SaveAction

	if saveAction == "" {
		saveAction = PostFormActionAutosave
	}

	var action = forms.FormSaveDefault(true)

	// a draft that was saved reports it to the page without leaving it
	draftSaved := func(action forms.FormSaveAction) forms.FormSaveAction {
		f.AddTemplateData("DraftSaved", true)

		return formhelpers.Retarget(
			formhelpers.Trigger(
				action,
				gin.H{"draft_saved": gin.H{"url": links.Link("post", post.ID, "edit_preview", "true")}},
			),
			"#last_draft_save",
		)
	}

	switch {
	case saveAction == PostFormActionPublish:
		// let's redirect to the post whenever we publish a post
		action = forms.FormSaveRedirect(links.Link("post", post.ID))
	case saved.Created:
		action = draftSaved(formhelpers.ReplaceHistory(action, links.Link("edit_post", post.ID)))
	case saveAction == PostFormActionMakeDraft:
	case saveAction != PostFormActionAutosave && post.PublishedAt.Valid:
		action = forms.FormSaveRedirect(links.Link("post", post.ID))
	default:
		action = draftSaved(action)
	}

	f.AddTemplateData("PostID", post.ID)
	f.AddTemplateData("IsPublished", post.PublishedAt.Valid)
	f.AddTemplateData("LastUpdatedAt", post.UpdatedAt.Time)

	return action, nil
}
