package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // postgres db driver
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func main() { //nolint:typecheck
	db := sqlx.MustConnect("postgres", os.Getenv("DATABASE_URL")+"?sslmode=disable")
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	enable := flag.Bool("enable", false, "enable registration")

	flag.Parse()

	ctx := context.Background()

	err := transact.Transact(db, func(tx *sql.Tx) error {
		settings, err := core.SystemSettings().One(ctx, tx)

		if err != nil {
			return err
		}

		settings.RegistrationOpen = *enable
		_, err = settings.Update(ctx, tx, boil.Infer())

		return err
	})

	if err != nil {
		panic(err)
	}
}
