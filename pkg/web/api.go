package web

import (
	"database/sql"
	"time"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

const GetPostsLimitMax = 100

type ApiPost struct {
	ID          string              `json:"id"`
	Subject     string              `json:"subject"`
	MdBody      string              `json:"md_body"`
	Visibility  core.PostVisibility `json:"visibility"`
	IsPublished bool                `json:"is_published"`
	PublishedAt int64               `json:"published_at,omitempty"`
	UpdatedAt   int64               `json:"updated_at,omitempty"`
	PublicURL   string              `json:"public_url"`
}

type ApiGetPostsResponse struct {
	Posts  []*ApiPost `json:"posts"`
	Cursor string     `json:"cursor"`
}

func ApiGetPosts(c *gin.Context, db *sqlx.DB, userID string) mo.Result[*ApiGetPostsResponse] {
	var input struct {
		UpdatedSince int64  `form:"updated_since"`
		Cursor       string `form:"cursor"`
		Limit        int    `form:"limit"`
	}

	if err := c.ShouldBind(&input); err != nil {
		return mo.Err[*ApiGetPostsResponse](err)
	}

	switch {
	case input.Limit <= 0:
		input.Limit = 1
	case input.Limit > GetPostsLimitMax:
		input.Limit = GetPostsLimitMax
	}

	q := []qm.QueryMod{
		core.PostWhere.UserID.EQ(userID),
		qm.OrderBy("id desc"),
		// +1 here is to understand whether it makes sense to fill cursor value,
		// we're discarding the last record otherwise
		qm.Limit(input.Limit + 1),
	}

	if input.UpdatedSince > 0 {
		t := time.Unix(input.UpdatedSince, 0)
		q = append(q, core.PostWhere.UpdatedAt.GT(null.TimeFrom(t)))
	}

	if input.Cursor != "" {
		q = append(q, core.PostWhere.ID.LT(input.Cursor))
	}

	posts, err := core.Posts(q...).All(c, db)

	if err != nil {
		return mo.Err[*ApiGetPostsResponse](err)
	}

	if len(posts) == 0 {
		return mo.Ok(&ApiGetPostsResponse{})
	}

	var newCursor string

	postLen := len(posts)

	if len(posts) > input.Limit {
		postLen--
		newCursor = posts[input.Limit-1].ID
	}

	return mo.Ok(&ApiGetPostsResponse{
		Posts: lo.Map(posts[0:postLen], func(p *core.Post, idx int) *ApiPost {
			var publishedAt int64

			if p.PublishedAt.Valid {
				publishedAt = p.PublishedAt.Time.Unix()
			}

			return &ApiPost{
				ID:          p.ID,
				Subject:     p.Subject,
				MdBody:      p.Body,
				Visibility:  p.VisibilityRadius,
				IsPublished: p.PublishedAt.Valid,
				PublishedAt: publishedAt,
				UpdatedAt:   p.UpdatedAt.Time.Unix(),
				PublicURL:   links.AbsLink("post", p.ID),
			}
		}),
		Cursor: newCursor,
	})
}

type ApiNewPostResponse struct {
	ID        string `json:"id"`
	PublicURL string `json:"public_url"`
}

func ApiNewPost(c *gin.Context, db *sqlx.DB, dbUser *core.User) mo.Result[*ApiNewPostResponse] {
	var input ApiPost

	if err := c.BindJSON(&input); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	// everything you see there is one big hack
	// to avoid duplicating business logic
	action := forms.PostFormActionSavePost

	if input.IsPublished {
		action = forms.PostFormActionPublish
	}

	form := forms.NewPostFormNew(dbUser)
	form.Input = &forms.PostFormInput{
		Subject:    input.Subject,
		Body:       input.MdBody,
		Visibility: input.Visibility,
		SaveAction: action,
	}

	if err := form.Validate(c, db); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	if err := transact.Transact(db, func(tx *sql.Tx) error {
		var err error
		_, err = form.Save(c, tx)

		return err
	}); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	tdata := form.TemplateData()
	postID := tdata["PostID"].(string)
	return mo.Ok(&ApiNewPostResponse{
		// and this only means that forms were not made with apis in mind
		ID:        postID,
		PublicURL: links.AbsLink("post", postID),
	})
}

func ApiEditPost(c *gin.Context, db *sqlx.DB, dbUser *core.User, postID string) mo.Result[*ApiNewPostResponse] {
	var input ApiPost

	if err := c.BindJSON(&input); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	// everything you see there is one big hack
	// to avoid duplicating business logic
	action := forms.PostFormActionMakeDraft

	if input.IsPublished {
		action = forms.PostFormActionPublish
	}

	form, err := forms.EditPostFormNew(c, db, dbUser, postID)

	if err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	form.Input = &forms.PostFormInput{
		Subject:    input.Subject,
		Body:       input.MdBody,
		Visibility: input.Visibility,
		SaveAction: action,
	}

	if err := form.Validate(c, db); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	if err := transact.Transact(db, func(tx *sql.Tx) error {
		var err error
		_, err = form.Save(c, tx)

		return err
	}); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	return mo.Ok(&ApiNewPostResponse{
		// and this only means that forms were not made with apis in mind
		ID:        postID,
		PublicURL: links.AbsLink("post", postID),
	})
}

func ApiDeletePost(c *gin.Context, db *sqlx.DB, dbUser *core.User, postID string) mo.Result[any] {
	err := postops.DeletePost(c, db, postID)

	if err != nil {
		return mo.Err[any](err)
	}

	return mo.Ok[any](nil)
}

type ApiUploadImageResponse struct {
	ImageID string `json:"image_id"`
}

func ApiUploadImage(c *gin.Context, db *sqlx.DB, dbUser *core.User, mediaServer media.MediaServer) mo.Result[*ApiUploadImageResponse] {
	file, err := c.FormFile("file")

	if err != nil {
		return mo.Err[*ApiUploadImageResponse](err)
	}

	f, err := file.Open()

	if err != nil {
		return mo.Err[*ApiUploadImageResponse](err)
	}

	var fname string

	err = transact.Transact(db, func(tx *sql.Tx) error {
		fname, err = media.HandleUpload(c, db, mediaServer, dbUser.ID, f)
		return err
	})

	if err != nil {
		return mo.Err[*ApiUploadImageResponse](err)
	}

	return mo.Ok(&ApiUploadImageResponse{
		ImageID: fname,
	})
}
