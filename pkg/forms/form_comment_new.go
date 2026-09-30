package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type NewCommentFormInput struct {
	Body    string `form:"body"`
	PostID  string `form:"post_id"`
	ReplyTo string `form:"reply_to"`
}

type NewCommentForm struct {
	*forms.FormBase[NewCommentFormInput]
	User  *core.User
	Posts *posts.Service
}

func NewCommentFormNew(svc *posts.Service, u *core.User, postID string) forms.Form {
	var form forms.Form = &NewCommentForm{
		FormBase: &forms.FormBase[NewCommentFormInput]{
			Name:                "new_comment",
			FormTemplate:        "form--comment.html",
			KeepValuesAfterSave: true,
			Input:               &NewCommentFormInput{},
			ExtraTemplateData: map[string]any{
				"User":   u,
				"PostID": postID,
			},
		},
		User:  u,
		Posts: svc,
	}

	return form
}

func (f *NewCommentForm) Validate(c *gin.Context, _ boil.ContextExecutor) error {
	if err := posts.ValidateCommentBody(f.Input.Body); err != nil {
		f.AddError("body", err.Error())
	}

	if err := f.Posts.CheckComment(c, f.User, f.Input.PostID, f.Input.ReplyTo); err != nil {
		return err
	}

	return f.Errors.PassedValidation()
}

func (f *NewCommentForm) Save(c context.Context, _ boil.ContextExecutor) (forms.FormSaveAction, error) {
	err := f.Posts.AddComment(c, f.User, posts.CommentInput{
		Body:    f.Input.Body,
		PostID:  f.Input.PostID,
		ReplyTo: f.Input.ReplyTo,
	})
	if err != nil {
		return nil, err
	}

	// XXX: we should focus on the comment
	return forms.FormSaveFullReload, nil
}
