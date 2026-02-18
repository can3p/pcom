package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SendInviteFormInput struct {
	Email string `form:"email"`
}

type SendInviteForm struct {
	*forms.FormBase[SendInviteFormInput]
	Sender sender.Sender
	User   *core.User
}

func SendInviteFormNew(sender sender.Sender, u *core.User) forms.Form {
	var form forms.Form = &SendInviteForm{
		FormBase: &forms.FormBase[SendInviteFormInput]{
			Name:         "send_invite",
			FormTemplate: "form--send-invite.html",
			Input:        &SendInviteFormInput{},
			ExtraTemplateData: map[string]any{
				"User": u,
			},
		},
		Sender: sender,
		User:   u,
	}

	return form
}

func (f *SendInviteForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	} else if err := mail.Validate(c, db, f.Input.Email); err != nil {
		f.AddError("email", err.Error())
	}

	return f.Errors.PassedValidation()
}

func (f *SendInviteForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	if err := mail.SendInvite(c, exec, f.Sender, f.User, f.Input.Email); err != nil {
		return nil, err
	}

	return forms.FormSaveFullReload, nil
}
