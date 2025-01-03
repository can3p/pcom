package forms

import (
	"context"
	"database/sql"
	"log"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type WhitelistConnectionInput struct {
	Username string `form:"uname"`
}

type WhitelistConnection struct {
	*forms.FormBase[WhitelistConnectionInput]
	User *core.User
}

func WhitelistConnectionNew(u *core.User) forms.Form {
	var form forms.Form = &WhitelistConnection{
		FormBase: &forms.FormBase[WhitelistConnectionInput]{
			Name:                "whitelist_connection",
			FormTemplate:        "form--whitelist-connection.html",
			KeepValuesAfterSave: true,
			Input:               &WhitelistConnectionInput{},
			ExtraTemplateData: map[string]interface{}{
				"User": u,
			},
		},
		User: u,
	}

	return form
}

func (f *WhitelistConnection) Validate(c *gin.Context, db boil.ContextExecutor) error {
	username := strings.ToLower(strings.TrimSpace(f.Input.Username))

	if username == "" {
		f.AddError("username", "username is a required attribute")
		return f.Errors.PassedValidation()
	} else if username == f.User.Username {
		f.AddError("username", "Can't add yourself to the list")
		return f.Errors.PassedValidation()
	}

	target, err := core.Users(core.UserWhere.Username.EQ(username)).One(c, db)

	if err == sql.ErrNoRows {
		f.AddError("username", "No such user")
		return f.Errors.PassedValidation()
	} else if err != nil {
		log.Printf("Failed to check username [%s] for existence on whitelist operation: %s", username, err.Error())
		f.AddError("username", "Failed to lookup the username")
		return f.Errors.PassedValidation()
	}

	isConnection, err := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(f.User.ID),
		core.UserConnectionWhere.User2ID.EQ(target.ID),
	).Exists(c, db)

	if err != nil {
		log.Printf("Failed to check username connection [%s] for existence on whitelist operation: %s", username, err.Error())
		f.AddError("username", "Failed to lookup the your connections")
		return f.Errors.PassedValidation()
	} else if isConnection {
		f.AddError("username", "You have this connection already")
		return f.Errors.PassedValidation()
	}

	// if connection id is not null, we already have one and that was an error in the previous check
	// @TODO: this should be upsert
	isWhitelisted, err := core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(f.User.ID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(target.ID),
		core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
	).Exists(c, db)

	if err != nil {
		log.Printf("Failed to check username connection [%s] for existence on whitelist operation: %s", username, err.Error())
		f.AddError("username", "Failed to lookup the your connections")
		return f.Errors.PassedValidation()
	} else if isWhitelisted {
		f.AddError("username", "Already in the list")
		return f.Errors.PassedValidation()
	}

	return f.Errors.PassedValidation()
}

func (f *WhitelistConnection) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	username := strings.ToLower(strings.TrimSpace(f.Input.Username))

	target, err := core.Users(core.UserWhere.Username.EQ(username)).One(c, exec)

	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()

	if err != nil {
		return nil, err
	}

	wlRequest := &core.WhitelistedConnection{
		ID:          id.String(),
		WhoID:       f.User.ID,
		AllowsWhoID: target.ID,
	}

	err = wlRequest.Insert(c, exec, boil.Infer())

	if err != nil {
		return nil, err
	}

	return forms.FormSaveFullReload, nil
}
