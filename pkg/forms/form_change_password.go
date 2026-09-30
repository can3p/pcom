package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

type ChangePasswordFormInput struct {
	OldPassword string `form:"old_password"`
	Password    string `form:"password"`
}

type ChangePasswordForm struct {
	*forms.FormBase[ChangePasswordFormInput]
	Accounts *accounts.Service
	User     *core.User
}

func ChangePasswordFormNew(accounts *accounts.Service, u *core.User) forms.Form {
	var form forms.Form = &ChangePasswordForm{
		FormBase: &forms.FormBase[ChangePasswordFormInput]{
			Name:         "change_password",
			FormTemplate: "form--settings-change-password.html",
			Input:        &ChangePasswordFormInput{},
		},
		Accounts: accounts,
		User:     u,
	}

	return form
}

func (f *ChangePasswordForm) Validate(c *gin.Context) error {
	if f.Input.Password == "" {
		f.AddError("password", "password is required")
		return forms.ErrValidationFailed
	}

	if f.Input.OldPassword == "" {
		f.AddError("old_password", "old password is required")
		return forms.ErrValidationFailed
	}

	if err := validation.ValidatePassword(f.Input.Password); err != nil {
		f.AddError("password", err.Error())
		return forms.ErrValidationFailed
	}

	if ok, _ := pgsession.CheckUserPwd(f.User.Pwdhash.String, f.User.Email, f.Input.OldPassword); !ok {
		f.AddError("old_password", "old password is not correct")
		return forms.ErrValidationFailed
	}

	return nil
}

func (f *ChangePasswordForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Accounts.ChangePassword(c, f.User, f.Input.OldPassword, f.Input.Password); err != nil {
		return nil, err
	}

	return f.FormBase.Save(c)
}
