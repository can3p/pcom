// Package logging carries pcom's one logger, a *zap.Logger. Long-running
// workers take it through a WithLogger option; handlers and services take it
// from the request context with FromContext. There is no global logger.
package logging

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The encoders --log-format chooses between.
const (
	FormatJSON    = "json"
	FormatConsole = "console"
)

type ctxKey struct{}

// ginKey stores the logger in a *gin.Context. gin's ContextWithFallback is
// off, so gin.Context.Value answers only string keys, from its own store.
const ginKey = "pcom.logging.logger"

// WithLogger returns ctx carrying logger. A *gin.Context is changed in place
// and returned, so the middleware after it and the handler see the logger;
// its request's context carries the logger too, for code called with
// c.Request.Context().
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	if gc, ok := ctx.(*gin.Context); ok {
		gc.Set(ginKey, logger)
		if gc.Request != nil {
			gc.Request = gc.Request.WithContext(context.WithValue(gc.Request.Context(), ctxKey{}, logger))
		}

		return ctx
	}

	return context.WithValue(ctx, ctxKey{}, logger)
}

// With adds fields to the logger ctx carries, as WithLogger does.
func With(ctx context.Context, fields ...zap.Field) context.Context {
	return WithLogger(ctx, FromContext(ctx).With(fields...))
}

// FromContext is the logger ctx carries, or a no-op logger when it carries
// none, so code called from a test or a CLI subcommand never needs a check.
// ctx may be a *gin.Context or a context.Context, also one derived from a
// *gin.Context.
func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.NewNop()
	}

	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		return l
	}

	if l, ok := ctx.Value(ginKey).(*zap.Logger); ok {
		return l
	}

	return zap.NewNop()
}

// New is a logger writing entries at level and above to w, encoded by format:
// FormatJSON for the deployed app, FormatConsole for a developer's terminal.
func New(level zapcore.Level, format string, w io.Writer) *zap.Logger {
	var enc zapcore.Encoder

	if format == FormatConsole {
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		enc = zapcore.NewConsoleEncoder(cfg)
	} else {
		enc = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	}

	return zap.New(zapcore.NewCore(enc, zapcore.AddSync(w), level), zap.AddCaller())
}
