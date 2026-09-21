package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/feedops/testutil"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/can3p/pcom/testcontainers/postgres"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func TestApiDeletePost_OnlyAuthorCanDelete(t *testing.T) {
	testDB, err := postgres.NewTestDB()
	require.NoError(t, err)
	defer func() { _ = testDB.Close() }()

	ctx := context.Background()

	author, err := testutil.CreateUser(ctx, testDB.DB, "author@example.com")
	require.NoError(t, err)

	stranger, err := testutil.CreateUser(ctx, testDB.DB, "stranger@example.com")
	require.NoError(t, err)

	post := &core.Post{
		ID:               uuid.NewString(),
		UserID:           author.ID,
		Body:             "hello",
		VisibilityRadius: core.PostVisibilityDirectOnly,
	}
	require.NoError(t, post.Insert(ctx, testDB.DB, boil.Infer()))

	postExists := func() bool {
		exists, err := core.PostExists(ctx, testDB.DB, post.ID)
		require.NoError(t, err)
		return exists
	}

	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+post.ID, nil)
		return c
	}

	res := web.ApiDeletePost(newContext(), testDB.DB, stranger, post.ID)
	require.True(t, res.IsError())
	require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
	require.True(t, postExists(), "a stranger must not be able to delete the post")

	res = web.ApiDeletePost(newContext(), testDB.DB, author, "0190a0a0-0000-7000-8000-000000000000")
	require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)

	res = web.ApiDeletePost(newContext(), testDB.DB, author, post.ID)
	require.NoError(t, res.Error())
	require.False(t, postExists(), "the author should be able to delete their post")
}
