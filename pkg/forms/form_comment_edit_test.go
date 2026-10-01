package forms_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestEditCommentForm_SaveAndValidate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	author := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	comment := testutil.Must(factory.Comment(ctx, db, post.ID, author.ID, factory.WithCommentBody("a typpo")))(t)

	newForm := func(body string) *forms.EditCommentForm {
		f, ok := forms.EditCommentFormNew(postsService(db, fakesender.New()), author, comment.ID).(*forms.EditCommentForm)
		require.True(t, ok)
		f.Input.Body = body

		return f
	}

	c, _ := newCtx(t)
	require.Error(t, newForm("").Validate(c), "an empty body is refused")

	form := newForm("a typo")
	require.NoError(t, form.Validate(c))
	action := testutil.Must(form.Save(ctx))(t)

	c, w := newCtx(t)
	action(c, form)
	require.NotEmpty(t, w.Header().Get("HX-Refresh"), "a save answers a full page reload")
}
