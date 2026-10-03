package forms

import (
	"context"
	"fmt"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type PostPromptFormInput struct {
	Message         string `form:"message"`
	RecipientHandle string `form:"recipient_handle"`
}

type PostPromptForm struct {
	*forms.FormBase[PostPromptFormInput]
	Posts             *posts.Service
	User              *core.User
	DirectConnections []*core.User
}

func PostPromptFormNew(svc *posts.Service, u *core.User, directConnections []*core.User) forms.Form {
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
		Posts:             svc,
		DirectConnections: directConnections,
	}

	return form
}

func (f *PostPromptForm) Validate(c *gin.Context) error {
	if err := f.Posts.ValidatePrompt(f.Input.Message); err != nil {
		return err
	}

	// this is a race condition, connection could be dropped in parallel to this,
	// we're fine with that
	if _, found := f.recipient(); !found {
		return fmt.Errorf("'%s' is not your direct connection", f.Input.RecipientHandle)
	}

	return f.Posts.CanPrompt(c, f.User)
}

// recipient is the direct connection the prompt is for.
func (f *PostPromptForm) recipient() (*core.User, bool) {
	return lo.Find(f.DirectConnections, func(u *core.User) bool {
		return u.Username == f.Input.RecipientHandle
	})
}

func (f *PostPromptForm) Save(c context.Context) (forms.FormSaveAction, error) {
	recipient, _ := f.recipient()

	if err := f.Posts.SendPrompt(c, f.User, recipient, f.Input.Message); err != nil {
		return nil, err
	}

	return f.FormBase.Save(c)
}
