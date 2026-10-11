package main

import (
	"fmt"
	"os"

	"github.com/can3p/pcom/pkg/logging"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database/sql driver
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newCLILogger is the logger of a subcommand other than serve: readable
// entries on stderr, so they never mix with what the command prints on
// stdout.
func newCLILogger() *zap.Logger {
	return logging.New(zapcore.InfoLevel, logging.FormatConsole, os.Stderr)
}

// openDB connects to the database; the caller runs the returned close on exit,
// which logs a failure to close through logger.
// fly.io's internal network does not use SSL: its DATABASE_URL says so.
func openDB(url string, logger *zap.Logger) (*sqlx.DB, func(), error) {
	db, err := sqlx.Connect("pgx", url)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to the database: %w", err)
	}

	return db, func() {
		if err := db.Close(); err != nil {
			logger.Warn("failed to close the database", zap.Error(err))
		}
	}, nil
}
