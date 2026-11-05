package logging

import (
	"context"
	"log/slog"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Slog returns a *slog.Logger that writes to z, for libraries that log with
// log/slog (River). Records below minLevel are dropped; attributes become zap
// fields.
func Slog(z *zap.Logger, minLevel zapcore.Level) *slog.Logger {
	return slog.New(&zapHandler{z: z, min: minLevel})
}

type zapHandler struct {
	z     *zap.Logger
	min   zapcore.Level
	group string
}

func (h *zapHandler) Enabled(_ context.Context, l slog.Level) bool {
	return zapLevel(l) >= h.min && h.z.Core().Enabled(zapLevel(l))
}

func (h *zapHandler) Handle(_ context.Context, r slog.Record) error {
	fields := make([]zap.Field, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, h.field(a))
		return true
	})

	if ce := h.z.Check(zapLevel(r.Level), r.Message); ce != nil {
		ce.Write(fields...)
	}

	return nil
}

func (h *zapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fields := make([]zap.Field, 0, len(attrs))
	for _, a := range attrs {
		fields = append(fields, h.field(a))
	}

	return &zapHandler{z: h.z.With(fields...), min: h.min, group: h.group}
}

func (h *zapHandler) WithGroup(name string) slog.Handler {
	g := name
	if h.group != "" {
		g = h.group + "." + name
	}

	return &zapHandler{z: h.z, min: h.min, group: g}
}

func (h *zapHandler) field(a slog.Attr) zap.Field {
	key := a.Key
	if h.group != "" {
		key = h.group + "." + key
	}

	return zap.Any(key, a.Value.Resolve().Any())
}

func zapLevel(l slog.Level) zapcore.Level {
	switch {
	case l >= slog.LevelError:
		return zapcore.ErrorLevel
	case l >= slog.LevelWarn:
		return zapcore.WarnLevel
	case l >= slog.LevelInfo:
		return zapcore.InfoLevel
	default:
		return zapcore.DebugLevel
	}
}
