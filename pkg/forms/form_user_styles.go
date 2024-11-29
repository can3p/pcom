package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SettingsUserStylesInput struct {
	Styles string `form:"styles"`
}

type SettingsUserStyles struct {
	*forms.FormBase[SettingsUserStylesInput]
	User *core.User
}

func SettingsUserStylesNew(u *core.User) *SettingsUserStyles {
	form := &SettingsUserStyles{
		FormBase: &forms.FormBase[SettingsUserStylesInput]{
			Name:                "settings_user_styles",
			FormTemplate:        "form--settings-user-styles.html",
			KeepValuesAfterSave: true,
			Input:               &SettingsUserStylesInput{},
			ExtraTemplateData:   map[string]interface{}{},
		},
		User: u,
	}

	return form
}

func (f *SettingsUserStyles) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateMinMax("styles", f.Input.Styles, 3, 10_000); err != nil {
		f.AddError("styles", err.Error())
	}

	return nil
}

func (f *SettingsUserStyles) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	styleID, err := uuid.NewV7()

	if err != nil {
		return nil, err
	}

	userStyle := core.UserStyle{
		ID:     styleID.String(),
		UserID: f.User.ID,
		Styles: f.Input.Styles,
	}

	if err := userStyle.Upsert(
		c, exec, true, []string{core.UserStyleColumns.UserID},
		boil.Whitelist(core.UserStyleColumns.UpdatedAt, core.UserStyleColumns.Styles),
		boil.Infer(),
	); err != nil {
		return nil, err
	}

	return f.FormBase.Save(c, exec)
}
