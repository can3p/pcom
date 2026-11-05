package logging_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/logging"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSlog_WritesToZapAboveTheLevel(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	l := logging.Slog(zap.New(core), zapcore.WarnLevel)

	l.Info("chatty")
	l.WithGroup("job").With("kind", "mail.x").Warn("slow", "attempt", 2)
	l.Error("broken")

	entries := logs.AllUntimed()
	require.Len(t, entries, 2)
	require.Equal(t, "slow", entries[0].Message)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	require.Equal(t, map[string]any{"job.kind": "mail.x", "job.attempt": int64(2)}, entries[0].ContextMap())
	require.Equal(t, zapcore.ErrorLevel, entries[1].Level)
}
