package forms

import (
	"context"
	"fmt"
	"slices"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/values"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
)

type SettingsGeneralFormInput struct {
	Timezone          string `form:"timezone"`
	ProfileVisibility string `form:"profile_visibility"`
	// EmailSubscriptions is the checkbox "let people without an account
	// subscribe by email"; an unchecked or disabled box sends nothing.
	EmailSubscriptions bool `form:"email_subscriptions_enabled"`
}

type SettingsGeneralForm struct {
	*forms.FormBase[SettingsGeneralFormInput]
	Accounts *accounts.Service
	User     *model.User
}

// audienceValues are the choices of "Who can read your journal", narrowest first.
var audienceValues = values.ValueList{
	{Label: "Your connections", Value: string(model.ProfileVisibilityConnections)},
	{Label: "Anyone with a pcom account", Value: string(model.ProfileVisibilityRegisteredUsers)},
	{Label: "Anyone on the internet", Value: string(model.ProfileVisibilityPublic)},
}

// SettingsGeneralFormNew builds the form for u. subscribers is how many
// confirmed email subscribers u has, which the page warns about before u
// switches subscriptions off or narrows the journal.
func SettingsGeneralFormNew(accounts *accounts.Service, u *model.User, subscribers int) *SettingsGeneralForm {
	form := &SettingsGeneralForm{
		FormBase: &forms.FormBase[SettingsGeneralFormInput]{
			Name:                "settings_general",
			FormTemplate:        "form--settings-general.html",
			KeepValuesAfterSave: true,
			Input:               &SettingsGeneralFormInput{},
			ExtraTemplateData: map[string]any{
				"User":              u,
				"ProfileVisibility": audienceValues,
				"Subscribers":       subscribers,
				"PublicValue":       string(model.ProfileVisibilityPublic),
			},
		},
		Accounts: accounts,
		User:     u,
	}

	return form
}

func (f *SettingsGeneralForm) Validate(c *gin.Context) error {
	if f.Input.Timezone == "" {
		f.AddError("timezone", "timezone is required")
		return forms.ErrValidationFailed
	}

	if f.Input.ProfileVisibility == "" {
		f.AddError("profile_visibility", "Profile visibility is required")
		return forms.ErrValidationFailed
	}

	found := slices.Contains(util.TimeZones, f.Input.Timezone)

	if !found {
		f.AddError("timezone", fmt.Sprintf("Cannot find the timezone [%s]", f.Input.Timezone))
		return forms.ErrValidationFailed
	}

	if err := model.ProfileVisibility(f.Input.ProfileVisibility).IsValid(); err != nil {
		f.AddError("profile_visibility", fmt.Sprintf("Invalid value [%s]", f.Input.ProfileVisibility))
		return forms.ErrValidationFailed
	}

	if f.Input.EmailSubscriptions && model.ProfileVisibility(f.Input.ProfileVisibility) != model.ProfileVisibilityPublic {
		f.AddError("email_subscriptions_enabled", "Email subscriptions need a journal that anyone on the internet can read.")
		return forms.ErrValidationFailed
	}

	return nil
}

func (f *SettingsGeneralForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Accounts.SaveGeneralSettings(c, f.User, f.Input.Timezone, model.ProfileVisibility(f.Input.ProfileVisibility), f.Input.EmailSubscriptions); err != nil {
		return nil, err
	}

	return f.FormBase.Save(c)
}
