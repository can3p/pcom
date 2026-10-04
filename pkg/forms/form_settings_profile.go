package forms

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
)

type SettingsProfileInput struct {
	About string `form:"about"`
}

// SettingsProfile edits the free-form "About" text shown on the user's journal.
type SettingsProfile struct {
	*forms.FormBase[SettingsProfileInput]
	Accounts *accounts.Service
	User     *core.User
}

func SettingsProfileNew(accounts *accounts.Service, u *core.User) *SettingsProfile {
	return &SettingsProfile{
		FormBase: &forms.FormBase[SettingsProfileInput]{
			Name:                "settings_profile",
			FormTemplate:        "form--settings-profile.html",
			KeepValuesAfterSave: true,
			Input:               &SettingsProfileInput{},
			ExtraTemplateData:   map[string]any{},
		},
		Accounts: accounts,
		User:     u,
	}
}

func (f *SettingsProfile) Validate(c *gin.Context) error {
	maxLength := f.Accounts.TextLimits().ProfileAboutMaxLength
	if n := utf8.RuneCountInString(strings.TrimSpace(f.Input.About)); n > maxLength {
		f.AddError("about", fmt.Sprintf("about text can have at most %d characters, this one has %d", maxLength, n))
	}

	return f.Errors.PassedValidation()
}

func (f *SettingsProfile) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Accounts.SaveProfile(c, f.User, f.Input.About); err != nil {
		return nil, err
	}

	return formhelpers.SuccessBadge("Profile has been saved successfully!"), nil
}
