package web

import (
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/samber/mo"
)

type WritePage struct {
	*BasePage
	Prompt             *postops.PostPrompt
	TranslationEnabled bool
}

// Write is the new post page. The prompt query parameter names the prompt the
// post answers; a prompt addressed to somebody else is ignored.
func Write(c *gin.Context, svc *posts.Service, userData *auth.UserData) mo.Result[*WritePage] {
	var prompt *postops.PostPrompt

	if promptID := c.Query("prompt"); promptID != "" {
		var err error

		prompt, err = svc.PromptFor(c, userData.DBUser, promptID)
		if err != nil {
			return mo.Err[*WritePage](err)
		}
	}

	return mo.Ok(&WritePage{
		BasePage: getBasePage(c, "New Post", userData),
		Prompt:   prompt,

		TranslationEnabled: svc.TranslationEnabled(),
	})
}

type EditPostPage struct {
	*BasePage
	PostID        string
	Input         forms.PostFormInput
	LastUpdatedAt time.Time
	IsPublished   bool
	Prompt        *postops.PostPrompt

	TranslationEnabled bool
}

// EditPost is the edit page of a post the viewer may edit.
func EditPost(c *gin.Context, svc *posts.Service, userData *auth.UserData, postID string) mo.Result[*EditPostPage] {
	view, err := svc.ForEdit(c, userData.DBUser, postID)
	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	post := view.Post

	var url string

	if post.R.URL != nil {
		url = post.R.URL.URL
	}

	return mo.Ok(&EditPostPage{
		BasePage: getBasePage(c, "Edit Post", userData),
		PostID:   post.ID,
		Input: forms.PostFormInput{
			Subject:    post.Subject.String,
			Body:       post.Body,
			Visibility: post.VisibilityRadius,
			URL:        url,

			AllowTranslation: post.AllowTranslation,
		},
		LastUpdatedAt: post.UpdatedAt.Time,
		IsPublished:   post.PublishedAt.Valid,
		Prompt:        view.Prompt,

		TranslationEnabled: svc.TranslationEnabled(),
	})
}
