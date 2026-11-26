package forms

import (
	"context"
	"errors"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/util/date"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type PostFormInput struct {
	Subject    string               `form:"subject"`
	URL        string               `form:"url"`
	Body       string               `form:"body"`
	Visibility model.PostVisibility `form:"visibility"`
	SaveAction PostFormAction       `form:"save_action"`
	// NotifyOffered says the form showed the "Email this post" checkbox,
	// so an unchecked NotifySubscribers means no rather than the default.
	NotifyOffered     bool `form:"notify_offered"`
	NotifySubscribers bool `form:"notify_subscribers"`
	// ScratchRev is the revision of the author's scratch pad that this form
	// last saw (see posts.SaveInput.ScratchRevision), 0 when it saw none.
	ScratchRev int `form:"scratch_rev"`
	// PostRev is the revision of the post that this form last saw (see
	// posts.SaveInput.PostRevision).
	PostRev int `form:"post_rev"`
	// NoteIDs are the notes inserted into a new post that are not marked as
	// used yet, comma separated; the notes picker keeps them in the form so
	// that they survive a reload.
	NoteIDs string `form:"note_ids"`
}

type PostForm struct {
	*forms.FormBase[PostFormInput]
	User   *model.User
	Posts  *posts.Service
	Post   *model.Post
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
	// PostFormActionTakeOver saves the text over the scratch pad or the post
	// that another tab changed.
	PostFormActionTakeOver PostFormAction = "take_over"
	// PostFormActionSaveCopy saves the text of a post that another tab
	// changed as a new draft.
	PostFormActionSaveCopy PostFormAction = "save_copy"
)

func NewPostFormNew(ctx context.Context, svc *posts.Service, u *model.User, promptID string) (*PostForm, error) {
	var prompt *postops.PostPrompt

	if promptID != "" {
		var err error

		prompt, err = svc.PromptFor(ctx, u, promptID)
		if err != nil {
			return nil, err
		}
	}

	reach, err := svc.SubscriberReach(ctx, u, nil)
	if err != nil {
		return nil, err
	}

	form := &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]any{
				"User":        u,
				"Prompt":      prompt,
				"Subscribers": reach,
			},
		},
		User:   u,
		Posts:  svc,
		Prompt: prompt,
	}

	return form, nil
}

func EditPostFormNew(ctx context.Context, svc *posts.Service, u *model.User, postID string) (*PostForm, error) {
	view, err := svc.ForEdit(ctx, u, postID)
	if errors.Is(err, service.ErrForbidden) {
		// somebody else's post is none of the actor's business
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	post := view.Post

	reach, err := svc.SubscriberReach(ctx, u, post)
	if err != nil {
		return nil, err
	}

	form := &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]any{
				"User":          u,
				"PostID":        post.ID,
				"IsPublished":   post.PublishedAt != nil,
				"LastUpdatedAt": lo.FromPtr(post.UpdatedAt),
				"Prompt":        view.Prompt,
				"Subscribers":   reach,
			},
		},
		User:   u,
		Posts:  svc,
		Post:   post,
		Prompt: view.Prompt,
	}

	return form, nil
}

// WithProofread tells the form whether the grammar check and style review are
// on: the editor then shows their buttons and panel, also when a validation
// error renders the form again.
func (f *PostForm) WithProofread(on bool) {
	f.AddTemplateData("Proofread", on)
}

// saveInput is what the form's input asks the service to save.
func (f *PostForm) saveInput() posts.SaveInput {
	in := posts.SaveInput{
		Subject:    f.Input.Subject,
		URL:        f.Input.URL,
		Body:       f.Input.Body,
		Visibility: f.Input.Visibility,
		Action:     posts.Action(f.Input.SaveAction),
		NoteIDs:    strings.Split(f.Input.NoteIDs, ","),
	}

	if f.Input.NotifyOffered {
		in.NotifySubscribers = new(f.Input.NotifySubscribers)
	}

	switch {
	case f.Post != nil:
		in.PostID = f.Post.ID
	case f.Prompt != nil:
		in.PromptID = f.Prompt.Prompt.ID
	}

	if f.Post == nil {
		in.ScratchRevision = f.Input.ScratchRev
	} else {
		in.PostRevision = f.Input.PostRev
	}

	return in
}

func (f *PostForm) Validate(c *gin.Context) error {
	limits := f.Posts.TextLimits()

	if err := validation.ValidateMinMax("subject", f.Input.Subject, 0, limits.PostSubjectMaxLength); err != nil {
		f.AddError("subject", err.Error())
	}

	if err := validation.ValidateURL(f.Input.URL); err != nil {
		f.AddError("url", err.Error())
	}

	if err := validation.ValidateMinMax("body", f.Input.Body, 0, limits.PostBodyMaxLength); err != nil {
		f.AddError("body", err.Error())
	}

	action := posts.Action(f.Input.SaveAction)
	if action == "" {
		action = posts.ActionAutosave
	}

	if err := validation.ValidateEnum(action,
		[]posts.Action{posts.ActionSavePost, posts.ActionMakeDraft, posts.ActionPublish, posts.ActionDelete, posts.ActionAutosave, posts.ActionTakeOver, posts.ActionSaveCopy},
		[]string{"Save Post", "Make draft", "Publish"}); err != nil {
		f.AddError("save_action", err.Error())
	}

	if err := validation.ValidateEnum(f.Input.Visibility,
		[]model.PostVisibility{model.PostVisibilityDirectOnly, model.PostVisibilitySecondDegree, model.PostVisibilityPublic},
		[]string{"direct only", "their connections as well", "public"}); err != nil {
		f.AddError("visibility", err.Error())
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

	if conflict, ok := errors.AsType[*posts.ScratchConflict](err); ok {
		return f.conflicted(c, conflict)
	} else if conflict, ok := errors.AsType[*posts.PostConflict](err); ok {
		return f.postConflicted(conflict), nil
	} else if err != nil {
		return nil, err
	}

	if saved.Deleted {
		return forms.FormSaveRedirect(links.Link("controls")), nil
	}

	if saved.Scratch != nil {
		return f.padSaved(saved.Scratch), nil
	}

	post := saved.Post
	saveAction := f.Input.SaveAction
	f.Input.PostRev = post.Revision

	if saveAction == PostFormActionSaveCopy {
		// the copy is another post: its editor is another page
		return forms.FormSaveRedirect(links.Link("edit_post", post.ID)), nil
	}

	if saveAction == "" {
		saveAction = PostFormActionAutosave
	}

	var action = forms.FormSaveDefault(true)

	// a draft that was saved reports it to the page without leaving it
	draftSaved := func(action forms.FormSaveAction) forms.FormSaveAction {
		f.AddTemplateData("DraftSaved", true)
		// a new post's Discard button now deletes the draft
		f.AddTemplateData("Promoted", saved.Created)

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
	case saveAction == PostFormActionSavePost && post.PublishedAt == nil:
		// "Save as Draft" swaps only the status, like an autosave, so the
		// textarea keeps its scroll and caret; the response also reports the
		// result next to the buttons (an out-of-band swap)
		f.AddTemplateData("DraftSavedByUser", true)

		if saved.Created {
			action = formhelpers.ReplaceHistory(action, links.Link("edit_post", post.ID))
		}

		action = draftSaved(action)
	case saved.Created:
		action = draftSaved(formhelpers.ReplaceHistory(action, links.Link("edit_post", post.ID)))
	case saveAction == PostFormActionMakeDraft:
	case saveAction == PostFormActionTakeOver:
		// the author goes on editing, as after an autosave
		action = draftSaved(action)
	case saveAction != PostFormActionAutosave && post.PublishedAt != nil:
		action = forms.FormSaveRedirect(links.Link("post", post.ID))
	default:
		action = draftSaved(action)
	}

	f.AddTemplateData("PostID", post.ID)
	f.AddTemplateData("IsPublished", post.PublishedAt != nil)
	f.AddTemplateData("LastUpdatedAt", lo.FromPtr(post.UpdatedAt))

	return action, nil
}

// conflictView is what the form's status shows when another tab or device
// wrote the scratch pad, or the post, since this form last saw it.
type conflictView struct {
	// OfPost is set for a conflict on a saved post, unset for one on the
	// scratch pad of a new post.
	OfPost bool
	// At is the time of the other write, as a clock time in the author's
	// zone; empty when it is not known.
	At string
	// PostID is the draft the other tab made from the text, if it did.
	PostID string
	// Emptied is set when the pad holds no text any more: the other tab saved
	// it as a draft, published it or discarded it. Otherwise it has newer
	// text, which this tab can load.
	Emptied bool
}

// padSaved answers an autosave of a new post, or a take over: only the
// status next to the title is swapped, with the pad's new revision.
func (f *PostForm) padSaved(pad *model.PostScratch) forms.FormSaveAction {
	f.Input.ScratchRev = pad.Revision
	f.AddTemplateData("DraftSaved", true)
	f.AddTemplateData("ScratchKeptAt", &pad.UpdatedAt)

	return formhelpers.Retarget(forms.FormSaveDefault(true), "#last_draft_save")
}

// conflicted answers a save that lost to another tab or device: nothing is
// written and the status tells the author, with the choices they have.
func (f *PostForm) conflicted(c context.Context, conflict *posts.ScratchConflict) (forms.FormSaveAction, error) {
	var promptID string
	if f.Prompt != nil {
		promptID = f.Prompt.Prompt.ID
	}

	pad, err := f.Posts.Scratch(c, f.User, promptID)
	if err != nil {
		return nil, err
	}

	view := &conflictView{
		PostID:  conflict.PromotedPostID,
		Emptied: pad != nil && pad.IsEmpty(),
	}

	if !conflict.At.IsZero() {
		view.At = date.LocalizeTime(f.User, conflict.At).Format("15:04")
	}

	f.AddTemplateData("DraftSaved", true)
	f.AddTemplateData("Conflict", view)

	return formhelpers.Retarget(forms.FormSaveDefault(true), "#last_draft_save"), nil
}

// postConflicted answers a save of a post that lost to another tab or device:
// nothing is written and the status tells the author, with the choices they
// have. The form keeps the revision it saw, so it goes on losing until the
// author picks one.
func (f *PostForm) postConflicted(conflict *posts.PostConflict) forms.FormSaveAction {
	view := &conflictView{OfPost: true}

	if !conflict.At.IsZero() {
		view.At = date.LocalizeTime(f.User, conflict.At).Format("15:04")
	}

	f.AddTemplateData("DraftSaved", true)
	f.AddTemplateData("Conflict", view)

	return formhelpers.Retarget(forms.FormSaveDefault(true), "#last_draft_save")
}
