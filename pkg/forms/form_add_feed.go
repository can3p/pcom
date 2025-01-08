package forms

import (
	"context"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type AddFeedFormInput struct {
	URL string `form:"url"`
}

type AddFeedForm struct {
	*forms.FormBase[AddFeedFormInput]
	User *core.User
}

func NewAddFeedForm(u *core.User) *AddFeedForm {
	return &AddFeedForm{
		FormBase: &forms.FormBase[AddFeedFormInput]{
			Name:         "add_rss_feed",
			FormTemplate: "form--settings-feeds.html",
			Input:        &AddFeedFormInput{},
		},
		User: u,
	}
}

func (f *AddFeedForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateURL(f.Input.URL); err != nil {
		f.AddError("url", err.Error())
	}

	if !strings.HasPrefix(f.Input.URL, "http://") && !strings.HasPrefix(f.Input.URL, "https://") {
		f.AddError("url", "url should have http or https protocol")
	}

	return f.Errors.PassedValidation()
}

func (f *AddFeedForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	url := strings.TrimSpace(f.Input.URL)

	if err := feedops.SubscribeToFeed(c, exec, f.User.ID, url); err != nil {
		return nil, err
	}

	return forms.FormSaveFullReload, nil
}
