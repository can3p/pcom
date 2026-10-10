package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/testutil/testlog"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestFromContext_NoLoggerIsNoop(t *testing.T) {
	ctx := context.Background()
	require.NotNil(t, logging.FromContext(ctx))
	require.False(t, logging.FromContext(ctx).Core().Enabled(zapcore.ErrorLevel))

	//nolint:staticcheck // a nil context must not panic either
	require.False(t, logging.FromContext(nil).Core().Enabled(zapcore.ErrorLevel))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.False(t, logging.FromContext(c).Core().Enabled(zapcore.ErrorLevel))
}

func TestWithLogger_PlainContext(t *testing.T) {
	logger, logs := testlog.New()

	ctx := logging.WithLogger(context.Background(), logger)
	logging.FromContext(ctx).Info("hello")

	require.Equal(t, 1, logs.FilterMessage("hello").Len())
}

// A *gin.Context carries the logger in place, and contexts derived from it
// see it, though gin's ContextWithFallback is off.
func TestWithLogger_GinContext(t *testing.T) {
	logger, logs := testlog.New()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.Same(t, c, logging.WithLogger(c, logger))

	derived, cancel := context.WithCancel(c)
	defer cancel()

	logging.FromContext(c).Info("from gin")
	logging.FromContext(derived).Info("from derived")

	require.Equal(t, 1, logs.FilterMessage("from gin").Len())
	require.Equal(t, 1, logs.FilterMessage("from derived").Len())
}

// Code called with c.Request.Context() sees the logger the middleware set,
// and fields added later.
func TestWithLogger_GinRequestContext(t *testing.T) {
	logger, logs := testlog.New()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	logging.WithLogger(c, logger)
	logging.With(c, zap.String("user_id", "u1"))
	logging.FromContext(c.Request.Context()).Info("from request")

	entries := logs.FilterMessage("from request").All()
	require.Len(t, entries, 1)
	require.Equal(t, "u1", entries[0].ContextMap()["user_id"])
}

func TestWith_AddsFields(t *testing.T) {
	ctx, logs := testlog.Context(context.Background())

	ctx = logging.With(ctx, zap.String("user_id", "u1"))
	logging.FromContext(ctx).Info("hello")

	require.Equal(t, "u1", logs.All()[0].ContextMap()["user_id"])
}

func TestNew_LevelAndFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(zapcore.WarnLevel, logging.FormatJSON, &buf)

	logger.Info("dropped")
	logger.Warn("kept", zap.Int("n", 1))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "kept", entry["msg"])
	require.EqualValues(t, 1, entry["n"])

	buf.Reset()
	logging.New(zapcore.InfoLevel, logging.FormatConsole, &buf).Info("plain")
	require.Error(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Contains(t, buf.String(), "plain")
}

// serve runs one request through Middleware, Recovery and handler.
func serve(t *testing.T, logger *zap.Logger, handle gin.RecoveryFunc, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	r := gin.New()
	r.Use(logging.Middleware(logger), logging.Recovery(handle))
	r.GET("/p", handler)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p?q=1", nil))

	return rec
}

func abort500(c *gin.Context, _ any) { c.AbortWithStatus(http.StatusInternalServerError) }

func TestMiddleware_LogsEachRequestWithItsID(t *testing.T) {
	logger, logs := testlog.New()

	handler := func(c *gin.Context) {
		logging.With(c, zap.String("user_id", "u1"))
		logging.FromContext(c).Info("in handler")
		c.Status(http.StatusTeapot)
	}

	serve(t, logger, abort500, handler)
	serve(t, logger, abort500, handler)

	requests, inHandler := logs.FilterMessage("request").All(), logs.FilterMessage("in handler").All()
	require.Len(t, requests, 2)
	require.Len(t, inHandler, 2)

	for i, e := range requests {
		f := e.ContextMap()
		require.Equal(t, zapcore.InfoLevel, e.Level)
		require.Equal(t, http.MethodGet, f["method"])
		require.Equal(t, "/p", f["path"])
		require.EqualValues(t, http.StatusTeapot, f["status"])
		require.Equal(t, "u1", f["user_id"], "fields added after the middleware reach the request log")
		require.NotEmpty(t, f["request_id"])
		require.Equal(t, inHandler[i].ContextMap()["request_id"], f["request_id"], "the handler logs with the request's ID")
	}

	require.NotEqual(t, requests[0].ContextMap()["request_id"], requests[1].ContextMap()["request_id"])
}

func TestMiddleware_ServerErrorAtErrorLevel(t *testing.T) {
	logger, logs := testlog.New()

	serve(t, logger, abort500, func(c *gin.Context) {
		_ = c.Error(errors.New("boom"))
		c.Status(http.StatusBadGateway)
	})

	e := logs.FilterMessage("request").All()[0]
	require.Equal(t, zapcore.ErrorLevel, e.Level)
	require.Equal(t, []any{"boom"}, e.ContextMap()["errors"])
}

func TestRecovery_LogsPanicWithStack(t *testing.T) {
	logger, logs := testlog.New()

	var handled any
	rec := serve(t, logger, func(c *gin.Context, err any) {
		handled = err
		abort500(c, err)
	}, func(*gin.Context) { panic("kaboom") })

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "kaboom", handled)

	panics := logs.FilterMessage("panic recovered").All()
	require.Len(t, panics, 1)
	f := panics[0].ContextMap()
	require.Equal(t, zapcore.ErrorLevel, panics[0].Level)
	require.Equal(t, "kaboom", f["panic"])
	require.Contains(t, f["stack"], "logging_test.TestRecovery_LogsPanicWithStack")
	require.NotEmpty(t, f["request_id"])

	require.EqualValues(t, http.StatusInternalServerError, logs.FilterMessage("request").All()[0].ContextMap()["status"])
}

// A client that went away is nobody's error: no panic entry, no handler.
func TestRecovery_BrokenPipeNotLogged(t *testing.T) {
	logger, logs := testlog.New()

	called := false
	serve(t, logger, func(*gin.Context, any) { called = true }, func(*gin.Context) { panic(syscall.EPIPE) })

	require.False(t, called)
	require.Zero(t, logs.FilterMessage("panic recovered").Len())
}
