package forms

import (
	"context"
	"strings"

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

type NewCommentFormInput struct {
	Body    string `form:"body"`
	PostID  string `form:"post_id"`
	ReplyTo string `form:"reply_to"`
}

type NewCommentForm struct {
	*forms.FormBase[NewCommentFormInput]
	User *core.User
}

func NewCommentFormNew(u *core.User) forms.Form {
	var form forms.Form = &NewCommentForm{
		FormBase: &forms.FormBase[NewCommentFormInput]{
			Name:                "new_comment",
			FormTemplate:        "form--post.html",
			KeepValuesAfterSave: true,
			Input:               &NewCommentFormInput{},
			ExtraTemplateData: map[string]interface{}{
				"User": u,
			},
		},
		User: u,
	}

	return form
}

func (f *NewCommentForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if err := validation.ValidateMinMax("body", f.Input.Body, 3, 2_000); err != nil {
		f.AddError("body", err.Error())
	}

	post, err := core.Posts(
		core.PostWhere.ID.EQ(f.Input.PostID),
	).One(c, db)

	if err != nil {
		return err
	}

	connRadius, err := userops.GetConnectionRadius(c, db, f.User.ID, post.UserID)

	if err != nil {
		return err
	}

	capabilities := postops.GetPostCapabilities(f.User.ID, post.UserID, connRadius)

	if !capabilities.CanLeaveComments {
		return ginhelpers.ErrForbidden
	}

	if f.Input.ReplyTo != "" {
		exists, err := core.PostComments(
			core.PostCommentWhere.ID.EQ(f.Input.ReplyTo),
			core.PostCommentWhere.PostID.EQ(f.Input.PostID),
		).Exists(c, db)

		if err != nil {
			return err
		}

		if !exists {
			return ginhelpers.ErrNotFound
		}
	}

	return f.Errors.PassedValidation()
}

func (f *NewCommentForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	body := strings.TrimSpace(f.Input.Body)

	id, err := uuid.NewV7()

	if err != nil {
		return nil, err
	}

	// we really want time ordered uuids for data locality, pagination etc
	commentID := id.String()
	// always keep the id of the top comment in the thread for simpler queries
	topCommentID := commentID

	if f.Input.ReplyTo != "" {
		comment, err := core.PostComments(
			core.PostCommentWhere.ID.EQ(f.Input.ReplyTo),
		).One(c, exec)

		if err != nil {
			return nil, err
		}

		topCommentID = comment.TopCommentID.String
	}

	comment := &core.PostComment{
		ID:              commentID,
		UserID:          f.User.ID,
		Body:            body,
		PostID:          f.Input.PostID,
		ParentCommentID: null.NewString(f.Input.ReplyTo, f.Input.ReplyTo != ""),
		TopCommentID:    null.StringFrom(topCommentID),
	}

	if err := comment.Insert(c, exec, boil.Infer()); err != nil {
		return nil, err
	}

	statID, err := uuid.NewV7()

	if err != nil {
		return nil, err
	}

	postStat := &core.PostStat{
		ID:             statID.String(),
		PostID:         f.Input.PostID,
		CommentsNumber: 1,
	}

	if err := postStat.Upsert(
		c, exec, true, []string{core.PostStatColumns.PostID},
		boil.Whitelist(core.PostStatColumns.UpdatedAt, core.PostStatColumns.CommentsNumber),
		boil.Infer(),
		core.UpsertUpdateSet("comments_number = excluded.comments_number + 1"),
	); err != nil {
		return nil, err
	}

	// XXX: we should focus on the comment
	return forms.FormSaveRedirect(links.Link("post", f.Input.PostID)), nil
}
