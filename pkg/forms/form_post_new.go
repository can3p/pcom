package forms

import (
	"context"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type NewPostFormInput struct {
	Subject string `form:"subject"`
	Body    string `form:"body"`
}

type NewPostForm struct {
	*forms.FormBase[NewPostFormInput]
	User *core.User
}

func NewPostFormNew(u *core.User) forms.Form {
	var form forms.Form = &NewPostForm{
		FormBase: &forms.FormBase[NewPostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &NewPostFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User": u,
			},
		},
		User: u,
	}

	return form
}

func (f *NewPostForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateMinMax("subject", f.Input.Subject, 3, 40); err != nil {
		f.AddError("subject", err.Error())
	}

	if err := validation.ValidateMinMax("body", f.Input.Body, 3, 40); err != nil {
		f.AddError("body", err.Error())
	}

	return nil
}

func (f *NewPostForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	subject := strings.TrimSpace(f.Input.Subject)
	body := strings.TrimSpace(f.Input.Body)

	post := &core.Post{
		ID:      uuid.NewString(),
		Subject: subject,
		Body:    body,
		UserID:  f.User.ID,
	}

	if err := post.Insert(c, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return forms.FormSaveRedirect(links.Link("post", post.ID)), nil
}
