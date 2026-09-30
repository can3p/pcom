package web

import (
	"database/sql"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/gin-gonic/gin"
	"github.com/samber/mo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

type WritePage struct {
	*BasePage
	Prompt *postops.PostPrompt
}

func Write(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*WritePage] {
	dbUser := userData.DBUser
	var prompt *postops.PostPrompt
	var err error

	if promptID := c.Query("prompt"); promptID != "" {
		prompt, err = postops.GetPostPrompt(c, db,
			core.PostPromptWhere.RecipientID.EQ(dbUser.ID),
			core.PostPromptWhere.ID.EQ(promptID),
		)

		if err != nil {
			return mo.Err[*WritePage](err)
		}
	}

	writePage := &WritePage{
		BasePage: getBasePage(c, "New Post", userData),
		Prompt:   prompt,
	}

	return mo.Ok(writePage)
}

type EditPostPage struct {
	*BasePage
	PostID        string
	Input         forms.PostFormInput
	LastUpdatedAt time.Time
	IsPublished   bool
	Prompt        *postops.PostPrompt
}

func EditPost(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData, postID string) mo.Result[*EditPostPage] {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
	).One(c, db)

	if err == sql.ErrNoRows {
		return mo.Err[*EditPostPage](ginhelpers.ErrNotFound)
	} else if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	author := post.R.User
	title := "Edit Post"

	connectionRadius, err := userops.GetConnectionRadius(c, db, userData.DBUser.ID, author.ID)

	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	capabilities := postops.GetPostCapabilities(connectionRadius)

	if !capabilities.CanEdit {
		return mo.Err[*EditPostPage](ginhelpers.ErrForbidden)
	}

	prompt, err := postops.GetPostPrompt(c, db, core.PostPromptWhere.PostID.EQ(null.StringFrom(post.ID)))

	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	var url string

	if post.R.URL != nil {
		url = post.R.URL.URL
	}

	editPostPage := &EditPostPage{
		BasePage: getBasePage(c, title, userData),
		PostID:   post.ID,
		Input: forms.PostFormInput{
			Subject:    post.Subject.String,
			Body:       post.Body,
			Visibility: post.VisibilityRadius,
			URL:        url,
		},
		LastUpdatedAt: post.UpdatedAt.Time,
		IsPublished:   post.PublishedAt.Valid,
		Prompt:        prompt,
	}

	return mo.Ok(editPostPage)
}
