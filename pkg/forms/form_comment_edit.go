package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
)

type EditCommentFormInput struct {
	Body string `form:"body"`
}

type EditCommentForm struct {
	*forms.FormBase[EditCommentFormInput]
	User      *core.User
	Posts     *posts.Service
	CommentID string
}

func EditCommentFormNew(svc *posts.Service, u *core.User, commentID string) forms.Form {
	var form forms.Form = &EditCommentForm{
		FormBase: &forms.FormBase[EditCommentFormInput]{
			Name:                "edit_comment",
			FormTemplate:        "form--comment.html",
			KeepValuesAfterSave: true,
			Input:               &EditCommentFormInput{},
			ExtraTemplateData: map[string]any{
				"User":   u,
				"EditID": commentID,
			},
		},
		User:      u,
		Posts:     svc,
		CommentID: commentID,
	}

	return form
}

func (f *EditCommentForm) Validate(c *gin.Context) error {
	if err := posts.ValidateCommentBody(f.Input.Body); err != nil {
		f.AddError("body", err.Error())
	}

	return f.Errors.PassedValidation()
}

func (f *EditCommentForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Posts.EditComment(c, f.User, f.CommentID, f.Input.Body); err != nil {
		return nil, err
	}

	return forms.FormSaveFullReload, nil
}
