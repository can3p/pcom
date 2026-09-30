package forms

import (
	"context"
	"log"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

type AcceptInviteFormInput struct {
	Username string `form:"username"`
	Password string `form:"password"`
}

type AcceptInviteForm struct {
	*forms.FormBase[AcceptInviteFormInput]
	Accounts *accounts.Service
	Invite   *core.UserInvitation
}

func AcceptInviteFormNew(accounts *accounts.Service, invite *core.UserInvitation) forms.Form {
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

	if f.Input.Password == "" {
		f.AddError("password", "password is required")
	} else if err := validation.ValidatePassword(f.Input.Password); err != nil {
		f.AddError("password", err.Error())
	}

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

	user, err := f.Accounts.AcceptInvite(c, f.Invite, username, f.Input.Password)
	if err != nil {
		return nil, err
	}

	if err := auth.StartSession(c.(*gin.Context), user); err != nil {
		return nil, err
	}

	return forms.FormSaveRedirect(links.Link("controls")), nil
}
