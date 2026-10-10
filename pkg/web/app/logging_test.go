package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/testutil/testlog"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/stretchr/testify/require"
)

// Each request is logged once, with the ID of the user auth found.
func TestRouter_RequestLogCarriesUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	user, err := factory.User(ctx, db)
	require.NoError(t, err)
	key, err := factory.APIKey(ctx, db, user.ID)
	require.NoError(t, err)

	logger, logs := testlog.New()
	h := app.New(&app.Deps{DB: db, Sender: fakesender.New(), MediaStorage: fakestorage.New(), Logger: logger, Config: settingsConfig()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	req.Header.Set("Authorization", "Bearer "+key.APIKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/login", nil))

	requests := logs.FilterMessage("request").All()
	require.Len(t, requests, 2)

	api, anon := requests[0].ContextMap(), requests[1].ContextMap()
	require.Equal(t, user.ID, api["user_id"])
	require.Equal(t, "/api/v1/posts", api["path"])
	require.EqualValues(t, http.StatusOK, api["status"])
	require.NotContains(t, anon, "user_id")
	require.Equal(t, "/login", anon["path"])
}
