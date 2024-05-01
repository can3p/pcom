package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type AcceptInviteFormInput struct {
	Password string `form:"password"`
}

type AcceptInviteForm struct {
	*forms.FormBase[AcceptInviteFormInput]
	Sender sender.Sender
	Invite *core.UserInvitation
}

func AcceptInviteFormNew(sender sender.Sender, invite *core.UserInvitation) forms.Form {
	var form forms.Form = &AcceptInviteForm{
		FormBase: &forms.FormBase[AcceptInviteFormInput]{
			Name:         "accept_invite",
			FormTemplate: "form--accept-invite.html",
			Input:        &AcceptInviteFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"Invite": invite,
			},
		},
		Sender: sender,
		Invite: invite,
	}

	return form
}

func (f *AcceptInviteForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if f.Input.Password == "" {
		f.AddError("password", "password is required")
	} else if err := validation.ValidatePassword(f.Input.Password); err != nil {
		f.AddError("password", err.Error())
	}

	return f.Errors.PassedValidation()
}

func (f *AcceptInviteForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	if err := auth.AcceptInvite(c, exec, f.Sender, f.Invite, f.Input.Password); err != nil {
		return nil, err
	}

	if err := auth.Login(c.(*gin.Context), exec, f.Invite.InvitationEmail.String, f.Input.Password); err != nil {
		return nil, err
	}

	return forms.FormSaveRedirect("/controls/"), nil
}
