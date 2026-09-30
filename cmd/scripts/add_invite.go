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

	email := flag.String("email", "", "account email")
	num := flag.Int("num", 0, "number of invites")

	flag.Parse()

	if *email == "" {
		panic("email is required")
	}

	if err := accounts.New(store, nil, nil).AddInvites(context.Background(), *email, *num); err != nil {
		panic(err)
	}
}
