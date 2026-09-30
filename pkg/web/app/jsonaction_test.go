package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestJSONAction(t *testing.T) {
	t.Parallel()

	type input struct {
		Name string `json:"name"`
	}

	cases := []struct {
		name       string
		body       string
		fnErr      error
		wantStatus int
		wantBody   string
		wantInput  string
		wantCalled bool
	}{
		{name: "success", body: `{"name":"x"}`, wantStatus: http.StatusOK, wantBody: `{}`, wantInput: "x", wantCalled: true},
		{name: "bad input", body: `{`, wantStatus: http.StatusBadRequest, wantBody: `{"explanation":"Bad input: unexpected EOF"}`},
		{name: "fn error reported as is", body: `{"name":"x"}`, fnErr: errors.New("Operation Failed: boom"), wantStatus: http.StatusBadRequest, wantBody: `{"explanation":"Operation Failed: boom"}`, wantInput: "x", wantCalled: true},
		{name: "validation error shows its message", body: `{"name":"x"}`, fnErr: service.Invalid("name", "Name is taken"), wantStatus: http.StatusBadRequest, wantBody: `{"explanation":"Name is taken"}`, wantInput: "x", wantCalled: true},
		{name: "forbidden", body: `{"name":"x"}`, fnErr: fmt.Errorf("wrapped: %w", service.ErrForbidden), wantStatus: http.StatusBadRequest, wantBody: `{"explanation":"Operation not allowed"}`, wantInput: "x", wantCalled: true},
		{name: "not found", body: `{"name":"x"}`, fnErr: service.ErrNotFound, wantStatus: http.StatusBadRequest, wantBody: `{"explanation":"Not found"}`, wantInput: "x", wantCalled: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := ginctx.New(t, http.MethodPost, "/controls/action/x", strings.NewReader(tc.body))

			var got string
			var called bool
			h := jsonAction(&Deps{}, func(_ *gin.Context, _ *core.User, in input) error {
				called = true
				got = in.Name
				return tc.fnErr
			})
			h(c)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
			require.Equal(t, tc.wantInput, got)
			require.Equal(t, tc.wantCalled, called, "fn runs only on a body that binds")
		})
	}
}
