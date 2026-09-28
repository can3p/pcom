// Command seed fills a development database; see package seed.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	"github.com/can3p/pcom/pkg/testutil/seed"
	_ "github.com/joho/godotenv/autoload" // load .env like the app
	_ "github.com/lib/pq"                 // postgres db driver
)

func main() {
	reset := flag.Bool("reset", false, "truncate every table except migrations and system_settings first")
	flag.Parse()

	if err := realMain(*reset); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func realMain(reset bool) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	return seed.Run(context.Background(), db, os.Stdout, seed.Options{
		Reset:    reset,
		Getenv:   os.Getenv,
		SiteRoot: os.Getenv("SITE_ROOT"),
	})
}
