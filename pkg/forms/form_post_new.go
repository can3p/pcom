package forms

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type PostFormInput struct {
	Subject    string              `form:"subject"`
	Body       string              `form:"body"`
	Visibility core.PostVisibility `form:"visibility"`
	SaveAction PostFormAction      `form:"save_action"`
}

type PostForm struct {
	*forms.FormBase[PostFormInput]
	User          *core.User
	Sender        sender.Sender
	MediaReplacer types.Replacer[string]
	Post          *core.Post
	Prompt        *postops.PostPrompt
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

func NewPostFormNew(ctx context.Context, db boil.ContextExecutor, sender sender.Sender, u *core.User, mediaReplacer types.Replacer[string], promptID string) (*PostForm, error) {
	var prompt *postops.PostPrompt
	var err error

	if promptID != "" {
		prompt, err = postops.GetPostPrompt(ctx, db,
			core.PostPromptWhere.RecipientID.EQ(u.ID),
			core.PostPromptWhere.ID.EQ(promptID),
		)

		if err != nil {
			return nil, err
		}
	}

	var form *PostForm = &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User":   u,
				"Prompt": prompt,
			},
		},
		User:          u,
		Sender:        sender,
		MediaReplacer: mediaReplacer,
		Prompt:        prompt,
	}

	return form, nil
}

func EditPostFormNew(ctx context.Context, db boil.ContextExecutor, sender sender.Sender, u *core.User, mediaReplacer types.Replacer[string], postID string) (*PostForm, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
		core.PostWhere.UserID.EQ(u.ID),
	).One(ctx, db)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ginhelpers.ErrNotFound
		}

		return nil, err
	}

	prompt, err := postops.GetPostPrompt(ctx, db, core.PostPromptWhere.PostID.EQ(null.StringFrom(post.ID)))

	if err != nil {
		return nil, err
	}

	var form *PostForm = &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User":          u,
				"PostID":        post.ID,
				"IsPublished":   post.PublishedAt.Valid,
				"LastUpdatedAt": post.UpdatedAt.Time,
				"Prompt":        prompt,
			},
		},
		User:          u,
		Post:          post,
		Sender:        sender,
		MediaReplacer: mediaReplacer,
		Prompt:        prompt,
	}

	return form, nil
}

func (f *PostForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateMinMax("subject", f.Input.Subject, 0, 100); err != nil {
		f.AddError("subject", err.Error())
	}

	if err := validation.ValidateMinMax("body", f.Input.Body, 0, 20_000); err != nil {
		f.AddError("body", err.Error())
	}

	saveAction := f.Input.SaveAction

	if saveAction == "" {
		saveAction = PostFormActionAutosave
	}

	if err := validation.ValidateEnum(saveAction,
		[]PostFormAction{PostFormActionSavePost, PostFormActionMakeDraft, PostFormActionPublish, PostFormActionDelete, PostFormActionAutosave},
		[]string{"Save Post", "Make draft", "Publish"}); err != nil {
		f.AddError("visibility", err.Error())
	}

	if err := validation.ValidateEnum(f.Input.Visibility,
		[]core.PostVisibility{core.PostVisibilityDirectOnly, core.PostVisibilitySecondDegree, core.PostVisibilityPublic},
		[]string{"direct only", "their connections as well", "public"}); err != nil {
		f.AddError("visibility", err.Error())
	}

	// this sounds like too much, but this way
	// we put the permission logic into a single place
	// and do not rely on adhoc queries
	if f.Post != nil {
		post := f.Post

		radius, err := userops.GetConnectionRadius(c, db, f.User.ID, post.UserID)

		if err != nil {
			return err
		}

		capabilities := postops.GetPostCapabilities(radius)

		if !capabilities.CanEdit {
			return ginhelpers.ErrForbidden
		}
	}

	return f.Errors.PassedValidation()
}

func (f *PostForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	subject := strings.TrimSpace(f.Input.Subject)
	body := strings.TrimSpace(f.Input.Body)

	saveAction := f.Input.SaveAction

	if saveAction == "" {
		saveAction = PostFormActionAutosave
	}

	if f.Post != nil && f.Input.SaveAction == PostFormActionDelete {
		err := postops.DeletePost(c, exec, f.Post.ID)

		if err != nil {
			return nil, err
		}

		return forms.FormSaveRedirect(links.Link("controls")), nil
	}

	post := &core.Post{
		Subject:          subject,
		Body:             body,
		UserID:           f.User.ID,
		VisibilityRadius: f.Input.Visibility,
	}

	var action = forms.FormSaveDefault(true)

	sendPublishNotification := false

	if f.Post == nil {
		postID, err := uuid.NewV7()

		if err != nil {
			return nil, err
		}

		post.ID = postID.String()

		if saveAction == PostFormActionPublish {
			// not null value means a published post
			post.PublishedAt = null.TimeFrom(time.Now())

			action = forms.FormSaveRedirect(links.Link("post", post.ID))
			sendPublishNotification = true
		} else {
			f.AddTemplateData("DraftSaved", true)
			action = formhelpers.Retarget(
				formhelpers.Trigger(
					formhelpers.ReplaceHistory(action, links.Link("edit_post", post.ID)),
					gin.H{"draft_saved": gin.H{"url": links.Link("post", post.ID, "edit_preview", "true")}},
				),
				"#last_draft_save",
			)
		}

		if err := post.Insert(c, exec, boil.Infer()); err != nil {
			return nil, err
		}

		if f.Prompt != nil {
			dbPrompt := f.Prompt.Prompt

			dbPrompt.PostID = null.StringFrom(post.ID)

			if _, err := dbPrompt.Update(c, exec, boil.Infer()); err != nil {
				return nil, err
			}
		}

		f.AddTemplateData("PostID", post.ID)
		f.AddTemplateData("IsPublished", post.PublishedAt.Valid)
		f.AddTemplateData("LastUpdatedAt", post.UpdatedAt.Time)
	} else {
		post.ID = f.Post.ID

		if saveAction == PostFormActionMakeDraft {
			// not null value means a published post
			post.PublishedAt = null.Time{}
		} else if saveAction == PostFormActionPublish {
			post.PublishedAt = null.TimeFrom(time.Now())

			// let's redirect to the post whenever we publish a post
			action = forms.FormSaveRedirect(links.Link("post", post.ID))
			sendPublishNotification = true
		} else {
			post.PublishedAt = f.Post.PublishedAt

			if saveAction != PostFormActionAutosave && post.PublishedAt.Valid {
				action = forms.FormSaveRedirect(links.Link("post", post.ID))
			} else {
				f.AddTemplateData("DraftSaved", true)
				action = formhelpers.Retarget(
					formhelpers.Trigger(
						action,
						gin.H{"draft_saved": gin.H{"url": links.Link("post", post.ID, "edit_preview", "true")}},
					),
					"#last_draft_save",
				)
			}
		}

		if _, err := post.Update(c, exec, boil.Infer()); err != nil {
			return nil, err
		}

		f.AddTemplateData("PostID", post.ID)
		f.AddTemplateData("IsPublished", post.PublishedAt.Valid)
		f.AddTemplateData("LastUpdatedAt", post.UpdatedAt.Time)
	}

	if sendPublishNotification {
		if f.Prompt != nil && f.Prompt.Prompt.DismissedAt.IsZero() {
			dbPrompt := f.Prompt.Prompt

			dbPrompt.PostID = null.StringFrom(post.ID)
			dbPrompt.DismissedAt = null.TimeFrom(time.Now())

			if _, err := dbPrompt.Update(c, exec, boil.Infer()); err != nil {
				return nil, err
			}

			if err := mail.PostPromptAnswer(c, exec, f.Sender, f.Prompt.Author, f.User, post, dbPrompt); err != nil {
				return nil, err
			}
		}

		directConnectionsIDs, err := userops.GetDirectUserIDs(c, exec, post.UserID)

		if err != nil {
			return nil, err
		}

		connections, err := core.Users(
			core.UserWhere.ID.IN(directConnectionsIDs),
		).All(c, exec)

		if err != nil {
			return nil, err
		}

		for _, conn := range connections {
			if err := mail.NewPost(c, exec, f.Sender, f.MediaReplacer, f.User, conn, post); err != nil {
				return nil, err
			}
		}
	}

	return action, nil
}
