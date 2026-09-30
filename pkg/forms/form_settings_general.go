package forms

import (
	"context"
	"fmt"
	"slices"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/values"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SettingsGeneralFormInput struct {
	Timezone          string `form:"timezone"`
	ProfileVisibility string `form:"profile_visibility"`
}

type SettingsGeneralForm struct {
	*forms.FormBase[SettingsGeneralFormInput]
	Accounts *accounts.Service
	User     *core.User
}

func SettingsGeneralFormNew(accounts *accounts.Service, u *core.User) *SettingsGeneralForm {
	form := &SettingsGeneralForm{
		FormBase: &forms.FormBase[SettingsGeneralFormInput]{
			Name:                "settings_general",
			FormTemplate:        "form--settings-general.html",
			KeepValuesAfterSave: true,
			Input:               &SettingsGeneralFormInput{},
			ExtraTemplateData: map[string]any{
				"User":              u,
				"ProfileVisibility": values.ProfileVisibilityValues,
			},
		},
		Accounts: accounts,
		User:     u,
	}

	return form
}

func (f *SettingsGeneralForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
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

	if err := core.ProfileVisibility(f.Input.ProfileVisibility).IsValid(); err != nil {
		f.AddError("profile_visibility", fmt.Sprintf("Invalid value [%s]", f.Input.ProfileVisibility))
		return forms.ErrValidationFailed
	}

	return nil
}

func (f *SettingsGeneralForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	if err := f.Accounts.SaveGeneralSettings(c, f.User, f.Input.Timezone, core.ProfileVisibility(f.Input.ProfileVisibility)); err != nil {
		return nil, err
	}

	return f.FormBase.Save(c, exec)
}
