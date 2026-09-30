package main

import (
	"context"
	"flag"
	"log"

	_ "github.com/lib/pq" // postgres db driver

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
)

func main() { //nolint:typecheck
	store, closeDB, err := repo.ConnectScript()
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := closeDB(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	enable := flag.Bool("enable", false, "enable registration")

	flag.Parse()

	if err := accounts.New(store, nil, nil).SetRegistrationOpen(context.Background(), *enable); err != nil {
		panic(err)
	}
}
