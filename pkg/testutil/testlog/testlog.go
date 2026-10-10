// Package testlog records what code under test logs, so a test can assert on
// the entries. Use it rather than building a zap core in the test.
package testlog

import (
	"context"

	"github.com/can3p/pcom/pkg/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// New is a logger that records every entry, debug level included, and the
// entries it has recorded.
func New() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)

	return zap.New(core), logs
}

// Context is ctx carrying a recording logger, for code that takes its logger
// from the context, and the entries it has recorded.
func Context(ctx context.Context) (context.Context, *observer.ObservedLogs) {
	logger, logs := New()

	return logging.WithLogger(ctx, logger), logs
}
