package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
)

type SettingsUserStylesInput struct {
	Styles string `form:"styles"`
}

type SettingsUserStyles struct {
	*forms.FormBase[SettingsUserStylesInput]
	Accounts *accounts.Service
	User     *core.User
}

func SettingsUserStylesNew(accounts *accounts.Service, u *core.User) *SettingsUserStyles {
	form := &SettingsUserStyles{
		FormBase: &forms.FormBase[SettingsUserStylesInput]{
			Name:                "settings_user_styles",
			FormTemplate:        "form--settings-user-styles.html",
			KeepValuesAfterSave: true,
			Input:               &SettingsUserStylesInput{},
			ExtraTemplateData:   map[string]any{},
		},
		Accounts: accounts,
		User:     u,
	}

	return form
}

func (f *SettingsUserStyles) Validate(c *gin.Context) error {
	if err := validation.ValidateMinMax("styles", f.Input.Styles, 0, f.Accounts.TextLimits().UserStylesMaxLength); err != nil {
		f.AddError("styles", err.Error())
	}

	return f.Errors.PassedValidation()
}

func (f *SettingsUserStyles) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Accounts.SaveUserStyles(c, f.User, f.Input.Styles); err != nil {
		return nil, err
	}

	return formhelpers.SuccessBadge("Styles have been saved successfully!"), nil
}
