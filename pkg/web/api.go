package web

import (
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

// GetPostsLimitMax is the most posts one API call returns.
const GetPostsLimitMax = posts.ListMax

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

type ApiNewPostResponse struct {
	ID        string `json:"id"`
	PublicURL string `json:"public_url"`
}

type ApiUploadImageResponse struct {
	ImageID string `json:"image_id"`
}

func ApiGetPosts(c *gin.Context, svc *posts.Service, actor *core.User) mo.Result[*ApiGetPostsResponse] {
	var form struct {
		UpdatedSince int64  `form:"updated_since"`
		Cursor       string `form:"cursor"`
		Limit        int    `form:"limit"`
	}

	if err := c.ShouldBind(&form); err != nil {
		return mo.Err[*ApiGetPostsResponse](err)
	}

	listing, err := svc.List(c, actor, posts.ListInput{UpdatedSince: form.UpdatedSince, Cursor: form.Cursor, Limit: form.Limit})
	if err != nil {
		return mo.Err[*ApiGetPostsResponse](err)
	}

	if len(listing.Posts) == 0 {
		return mo.Ok(&ApiGetPostsResponse{})
	}

	return mo.Ok(&ApiGetPostsResponse{
		Posts: lo.Map(listing.Posts, func(p *core.Post, idx int) *ApiPost {
			var publishedAt int64

			if p.PublishedAt.Valid {
				publishedAt = p.PublishedAt.Time.Unix()
			}

			return &ApiPost{
				ID:          p.ID,
				Subject:     postops.PostSubject(p.Subject),
				MdBody:      p.Body,
				Visibility:  p.VisibilityRadius,
				IsPublished: p.PublishedAt.Valid,
				PublishedAt: publishedAt,
				UpdatedAt:   p.UpdatedAt.Time.Unix(),
				PublicURL:   svc.PostURL(p.ID),
			}
		}),
		Cursor: listing.Cursor,
	})
}

func ApiNewPost(c *gin.Context, svc *posts.Service, actor *core.User) mo.Result[*ApiNewPostResponse] {
	var input ApiPost

	if err := c.BindJSON(&input); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	action := posts.ActionSavePost

	if input.IsPublished {
		action = posts.ActionPublish
	}

	saved, err := svc.Save(c, actor, posts.SaveInput{
		Subject:    input.Subject,
		Body:       input.MdBody,
		Visibility: input.Visibility,
		Action:     action,
	})
	if err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	return mo.Ok(&ApiNewPostResponse{
		ID:        saved.Post.ID,
		PublicURL: svc.PostURL(saved.Post.ID),
	})
}

func ApiEditPost(c *gin.Context, svc *posts.Service, actor *core.User, postID string) mo.Result[*ApiNewPostResponse] {
	var input ApiPost

	if err := c.BindJSON(&input); err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	action := posts.ActionMakeDraft

	if input.IsPublished {
		action = posts.ActionPublish
	}

	_, err := svc.Save(c, actor, posts.SaveInput{
		PostID:     postID,
		Subject:    input.Subject,
		Body:       input.MdBody,
		Visibility: input.Visibility,
		Action:     action,
	})
	if err != nil {
		return mo.Err[*ApiNewPostResponse](err)
	}

	return mo.Ok(&ApiNewPostResponse{
		ID:        postID,
		PublicURL: svc.PostURL(postID),
	})
}

func ApiDeletePost(c *gin.Context, svc *posts.Service, actor *core.User, postID string) mo.Result[any] {
	if err := svc.Delete(c, actor, postID); err != nil {
		return mo.Err[any](err)
	}

	return mo.Ok[any](nil)
}

func ApiUploadImageWith(c *gin.Context, svc *posts.Service, actor *core.User) mo.Result[*ApiUploadImageResponse] {
	file, err := c.FormFile("file")
	if err != nil {
		return mo.Err[*ApiUploadImageResponse](err)
	}

	f, err := file.Open()
	if err != nil {
		return mo.Err[*ApiUploadImageResponse](err)
	}

	fname, err := svc.UploadImage(c, actor, f)
	if err != nil {
		return mo.Err[*ApiUploadImageResponse](err)
	}

	return mo.Ok(&ApiUploadImageResponse{
		ImageID: fname,
	})
}
