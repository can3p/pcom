package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type LoginFormInput struct {
	Email     string `form:"email"`
	Password  string `form:"password"`
	ReturnURL string `form:"return_url"`
	Sign      string `form:"sign"`
}

type LoginForm struct {
	*forms.FormBase[LoginFormInput]
}

func LoginFormNew() forms.Form {
	var form forms.Form = &LoginForm{
		FormBase: &forms.FormBase[LoginFormInput]{
			Name:         "login",
			FormTemplate: "form--login.html",
			Input:        &LoginFormInput{},
		},
	}

	return form
}

func (f *LoginForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if f.Input.Email == "" {
		f.AddError("email", "email is required")
		return forms.ErrValidationFailed
	}

	if f.Input.Password == "" {
		f.AddError("password", "password is required")
		return forms.ErrValidationFailed
	}

	return auth.CheckCredentials(c, db, f.Input.Email, f.Input.Password)
}

func (f *LoginForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	if err := auth.Login(c.(*gin.Context), exec, f.Input.Email, f.Input.Password); err != nil {
		return nil, err
	}

	if f.Input.ReturnURL != "" && auth.HashValue(f.Input.ReturnURL) == f.Input.Sign {
		return forms.FormSaveRedirect(util.SiteRoot() + f.Input.ReturnURL), nil
	}

	return forms.FormSaveRedirect(links.DefaultAuthorizedHome()), nil
}
