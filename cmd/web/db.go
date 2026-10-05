package main

import (
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database/sql driver
	"github.com/jmoiron/sqlx"
)

// openDB connects to the database; the caller runs the returned close on exit.
// fly.io's internal network does not use SSL: its DATABASE_URL says so.
func openDB(url string) (*sqlx.DB, func(), error) {
	db, err := sqlx.Connect("pgx", url)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to the database: %w", err)
	}

	return db, func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}, nil
}
