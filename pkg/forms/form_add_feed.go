package forms

import (
	"context"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
)

// FeedsSectionTemplate renders the settings' RSS feeds section, which holds
// the add form; a saved add form re-renders the whole section in its place.
const FeedsSectionTemplate = "partial--settings_feeds.html"

type AddFeedFormInput struct {
	URL string `form:"url"`
}

type AddFeedForm struct {
	*forms.FormBase[AddFeedFormInput]
	User  *model.User
	feeds *feeds.Service
}

func NewAddFeedForm(feeds *feeds.Service, u *model.User) *AddFeedForm {
	return &AddFeedForm{
		FormBase: &forms.FormBase[AddFeedFormInput]{
			Name:         "add_rss_feed",
			FormTemplate: "form--settings-feeds.html",
			Input:        &AddFeedFormInput{},
			ExtraTemplateData: map[string]any{
				"SavedMessage": "Feed added",
			},
		},
		User:  u,
		feeds: feeds,
	}
}

// FeedsSection is the data of FeedsSectionTemplate: the user's subscriptions,
// the user the times are shown for, and the add form's template data.
func FeedsSection(subs []*feeds.RssFeed, u *model.User, form map[string]any) map[string]any {
	return map[string]any{
		"Feeds":  subs,
		"DBUser": u,
		"Form":   form,
	}
}

func (f *AddFeedForm) Validate(c *gin.Context) error {
	if err := validation.ValidateURL(f.Input.URL); err != nil {
		f.AddError("url", err.Error())
	}

	if !strings.HasPrefix(f.Input.URL, "http://") && !strings.HasPrefix(f.Input.URL, "https://") {
		f.AddError("url", "url should have http or https protocol")
	}

	return f.Errors.PassedValidation()
}

// Save subscribes the user and answers with the feeds section, listing the
// new feed above an empty form that reports it was added.
func (f *AddFeedForm) Save(c context.Context) (forms.FormSaveAction, error) {
	url := strings.TrimSpace(f.Input.URL)

	if err := f.feeds.Subscribe(c, f.User, url); err != nil {
		return nil, err
	}

	subs, err := f.feeds.Subscriptions(c, f.User)
	if err != nil {
		return nil, err
	}

	f.FormSaved = true
	f.ClearInput()

	return formhelpers.Retarget(func(c *gin.Context, _ forms.Form) {
		c.HTML(http.StatusOK, FeedsSectionTemplate, FeedsSection(subs, f.User, f.TemplateData()))
	}, "#feeds"), nil
}
