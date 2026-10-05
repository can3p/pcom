package forms

import (
	"context"
	"log"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type AcceptInviteFormInput struct {
	Username string `form:"username"`
}

type AcceptInviteForm struct {
	*forms.FormBase[AcceptInviteFormInput]
	Accounts *accounts.Service
	Invite   *model.UserInvitation
}

func AcceptInviteFormNew(accounts *accounts.Service, invite *model.UserInvitation) forms.Form {
	var form forms.Form = &AcceptInviteForm{
		FormBase: &forms.FormBase[AcceptInviteFormInput]{
			Name:         "accept_invite",
			FormTemplate: "form--accept-invite.html",
			Input:        &AcceptInviteFormInput{},
			ExtraTemplateData: map[string]any{
				"Invite": invite,
			},
		},
		Accounts: accounts,
		Invite:   invite,
	}

	return form
}

func (f *AcceptInviteForm) Validate(c *gin.Context) error {
	username := strings.TrimSpace(strings.ToLower(f.Input.Username))

	if username == "" {
		f.AddError("username", "username is required")
	} else if err := validation.ValidateUsername(username); err != nil {
		f.AddError("username", err.Error())
	} else {
		exists, err := f.Accounts.UsernameTaken(c, username)

		if err != nil {
			log.Printf("Failed to check username [%s] for duplication: %s", username, err.Error())
			f.AddError("username", "internal error")
		} else if exists {
			f.AddError("username", "this username is not available")
		}
	}

	return f.Errors.PassedValidation()
}

func (f *AcceptInviteForm) Save(c context.Context) (forms.FormSaveAction, error) {
	username := strings.TrimSpace(strings.ToLower(f.Input.Username))

	gc := c.(*gin.Context)

	attemptID, err := f.Accounts.AcceptInvite(gc.Request.Context(), f.Invite, username)
	if err != nil {
		return nil, panicOnFatal(err)
	}

	return codeStep(gc, attemptID, lo.FromPtr(f.Invite.InvitationEmail))
}
