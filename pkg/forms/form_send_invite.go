package forms

import (
	"context"
	"errors"
	"net/http"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
)

// InvitesSectionTemplate renders the settings' invites section, which holds
// the send form; a sent invite re-renders the whole section in its place.
const InvitesSectionTemplate = "partial--settings_invites.html"

type SendInviteFormInput struct {
	Email string `form:"email"`
}

type SendInviteForm struct {
	*forms.FormBase[SendInviteFormInput]
	Accounts *accounts.Service
	User     *model.User
}

func SendInviteFormNew(accounts *accounts.Service, u *model.User) forms.Form {
	var form forms.Form = &SendInviteForm{
		FormBase: &forms.FormBase[SendInviteFormInput]{
			Name:         "send_invite",
			FormTemplate: "form--send-invite.html",
			Input:        &SendInviteFormInput{},
			ExtraTemplateData: map[string]any{
				"User":         u,
				"SavedMessage": "Invite sent",
			},
		},
		Accounts: accounts,
		User:     u,
	}

	return form
}

// InvitesSection is the data of InvitesSectionTemplate: the invites left, the
// ones sent and the send form's template data.
func InvitesSection(available int64, used []*model.UserInvitation, form map[string]any) map[string]any {
	return map[string]any{
		"Available": available,
		"Used":      used,
		"Form":      form,
	}
}

func (f *SendInviteForm) Validate(c *gin.Context) error {
	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	} else if err := fieldError(f, "email", f.Accounts.CheckInviteEmail(c, f.Input.Email)); err != nil {
		return err
	}

	return f.Errors.PassedValidation()
}

// Save sends the invite and answers with the invites section, listing the
// invitee and the invites left. A refusal the user can act on is shown under
// the field instead.
func (f *SendInviteForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Accounts.SendInvite(c, f.User, f.Input.Email); err != nil {
		var invalid *service.ValidationError
		if !errors.As(err, &invalid) {
			return nil, err
		}

		f.AddError("email", invalid.Message)

		return forms.FormSaveDefault(true), nil
	}

	view, err := f.Accounts.Settings(c, f.User)
	if err != nil {
		return nil, err
	}

	f.FormSaved = true
	f.ClearInput()

	return formhelpers.Retarget(func(c *gin.Context, _ forms.Form) {
		c.HTML(http.StatusOK, InvitesSectionTemplate, InvitesSection(view.AvailableInvites, view.UsedInvites, f.TemplateData()))
	}, "#invites"), nil
}
