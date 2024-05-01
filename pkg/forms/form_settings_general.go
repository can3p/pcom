package forms

import (
	"context"
	"fmt"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SettingsGeneralFormInput struct {
	Timezone string `form:"timezone"`
}

type SettingsGeneralForm struct {
	*forms.FormBase[SettingsGeneralFormInput]
	User *core.User
}

func SettingsGeneralFormNew(u *core.User) forms.Form {
	var form forms.Form = &SettingsGeneralForm{
		FormBase: &forms.FormBase[SettingsGeneralFormInput]{
			Name:                "settings_general",
			FormTemplate:        "form--settings-general.html",
			KeepValuesAfterSave: true,
			Input:               &SettingsGeneralFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User": u,
			},
		},
		User: u,
	}

	return form
}

func (f *SettingsGeneralForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if f.Input.Timezone == "" {
		f.AddError("timezone", "timezone is required")
		return forms.ErrValidationFailed
	}

	found := false
	for _, tz := range util.TimeZones {
		if tz == f.Input.Timezone {
			found = true
			break
		}
	}

	if !found {
		f.AddError("timezone", fmt.Sprintf("Cannot find the timezone [%s]", f.Input.Timezone))
		return forms.ErrValidationFailed
	}

	return nil
}

func (f *SettingsGeneralForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	f.User.Timezone = f.Input.Timezone

	if _, err := f.User.Update(c, exec, boil.Whitelist(
		core.UserColumns.Timezone,
		core.UserColumns.UpdatedAt,
	)); err != nil {
		return nil, errors.Wrapf(err, "failed to save to the db")
	}

	return f.FormBase.Save(c, exec)
}
