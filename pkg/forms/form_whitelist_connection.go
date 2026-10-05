package forms

import (
	"context"
	"errors"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/gin-gonic/gin"
)

type WhitelistConnectionInput struct {
	Username string `form:"uname"`
}

type WhitelistConnection struct {
	*forms.FormBase[WhitelistConnectionInput]
	User        *model.User
	Connections *connections.Service
}

func WhitelistConnectionNew(u *model.User, conns *connections.Service) forms.Form {
	var form forms.Form = &WhitelistConnection{
		FormBase: &forms.FormBase[WhitelistConnectionInput]{
			Name:                "whitelist_connection",
			FormTemplate:        "form--whitelist-connection.html",
			KeepValuesAfterSave: true,
			Input:               &WhitelistConnectionInput{},
			ExtraTemplateData: map[string]any{
				"User": u,
			},
		},
		User:        u,
		Connections: conns,
	}

	return form
}

func (f *WhitelistConnection) Validate(c *gin.Context) error {
	var invalid *service.ValidationError

	if err := f.Connections.CheckWhitelist(c, f.User, f.Input.Username); errors.As(err, &invalid) {
		f.AddError(invalid.Field, invalid.Message)
	} else if err != nil {
		return err
	}

	return f.Errors.PassedValidation()
}

func (f *WhitelistConnection) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Connections.Whitelist(c, f.User, f.Input.Username); err != nil {
		return nil, err
	}

	return forms.FormSaveFullReload, nil
}
