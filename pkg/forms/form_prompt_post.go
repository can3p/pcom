package forms

import (
	"context"
	"fmt"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type PostPromptFormInput struct {
	Message         string `form:"message"`
	RecipientHandle string `form:"recipient_handle"`
}

type PostPromptForm struct {
	*forms.FormBase[PostPromptFormInput]
	Sender            sender.Sender
	User              *core.User
	DirectConnections []*core.User
}

func PostPromptFormNew(sender sender.Sender, u *core.User, directConnections []*core.User) forms.Form {
	var form forms.Form = &PostPromptForm{
		FormBase: &forms.FormBase[PostPromptFormInput]{
			Name:                "new_comment",
			FormTemplate:        "form--post-prompt.html",
			KeepValuesAfterSave: true,
			Input:               &PostPromptFormInput{},
			ExtraTemplateData: map[string]any{
				"User":              u,
				"DirectConnections": directConnections,
			},
		},
		User:              u,
		Sender:            sender,
		DirectConnections: directConnections,
	}

	return form
}

func (f *PostPromptForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateMinMax("message", f.Input.Message, 3, 1400); err != nil {
		return err
	}

	// this is a race condition, connection could be dropped in parallel to this,
	// we're fine with that
	if _, found := lo.Find(f.DirectConnections, func(u *core.User) bool {
		return u.Username == f.Input.RecipientHandle
	}); !found {
		return fmt.Errorf("'%s' is not your direct connection", f.Input.RecipientHandle)
	}

	return postops.CanPromptNow(c, db, f.User.ID)
}

func (f *PostPromptForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	message := strings.TrimSpace(f.Input.Message)

	id, err := uuid.NewV7()

	if err != nil {
		return nil, err
	}

	recipient, _ := lo.Find(f.DirectConnections, func(u *core.User) bool {
		return u.Username == f.Input.RecipientHandle
	})

	postPrompt := &core.PostPrompt{
		ID:          id.String(),
		AskerID:     f.User.ID,
		Message:     message,
		RecipientID: recipient.ID,
	}

	if err := postPrompt.Insert(c, exec, boil.Infer()); err != nil {
		return nil, err
	}

	if err := mail.PostPrompt(c, exec, f.Sender, f.User, recipient, postPrompt); err != nil {
		return nil, err
	}

	return f.FormBase.Save(c, exec)
}
