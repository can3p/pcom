package forms

import (
	"context"
	"strings"
	"time"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type PostFormInput struct {
	Subject    string `form:"subject"`
	Body       string `form:"body"`
	Visibility string `form:"visibility"`
}

type PostForm struct {
	*forms.FormBase[PostFormInput]
	User   *core.User
	PostID string
}

func NewPostFormNew(u *core.User) forms.Form {
	var form forms.Form = &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User": u,
			},
		},
		User: u,
	}

	return form
}

func EditPostFormNew(u *core.User, postID string) forms.Form {
	var form forms.Form = &PostForm{
		FormBase: &forms.FormBase[PostFormInput]{
			Name:                "new_post",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &PostFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User":   u,
				"PostID": postID,
			},
		},
		User:   u,
		PostID: postID,
	}

	return form
}

func (f *PostForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateMinMax("subject", f.Input.Subject, 3, 100); err != nil {
		f.AddError("subject", err.Error())
	}

	if err := validation.ValidateMinMax("body", f.Input.Body, 3, 20_000); err != nil {
		f.AddError("body", err.Error())
	}

	if err := validation.ValidateEnum(f.Input.Visibility,
		[]string{core.PostVisibilityDirectOnly.String(), core.PostVisibilitySecondDegree.String()},
		[]string{"direct only", "their connections as well"}); err != nil {
		f.AddError("visibility", err.Error())
	}

	// this sounds like too much, but this way
	// we put the permission logic into a single place
	// and do not rely on adhoc queries
	if f.PostID != "" {
		post, err := core.Posts(
			core.PostWhere.ID.EQ(f.PostID),
		).One(c, db)

		if err != nil {
			return err
		}

		radius, err := userops.GetConnectionRadius(c, db, f.User.ID, post.UserID)

		if err != nil {
			return err
		}

		capabilities := postops.GetPostCapabilities(f.User.ID, post.UserID, radius)

		if !capabilities.CanEdit {
			return ginhelpers.ErrForbidden
		}
	}

	return f.Errors.PassedValidation()
}

func (f *PostForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	subject := strings.TrimSpace(f.Input.Subject)
	body := strings.TrimSpace(f.Input.Body)

	post := &core.Post{
		Subject:          subject,
		Body:             body,
		UserID:           f.User.ID,
		VisibilityRadius: core.PostVisibility(f.Input.Visibility),
	}

	if f.PostID == "" {
		postID, err := uuid.NewV7()

		if err != nil {
			return nil, err
		}

		post.ID = postID.String()
		// not null value means a published post
		post.PublishedAt = null.TimeFrom(time.Now())

		if err := post.Insert(c, exec, boil.Infer()); err != nil {
			return nil, err
		}
	} else {
		post.ID = f.PostID

		if _, err := post.Update(c, exec, boil.Infer()); err != nil {
			return nil, err
		}
	}

	return forms.FormSaveRedirect(links.Link("post", post.ID)), nil
}
