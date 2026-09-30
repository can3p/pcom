package main

import (
	"context"
	"os"

	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/testutil/seed"
)

type seedCmd struct {
	config.Seed
}

func (c *seedCmd) Execute([]string) error {
	db, closeDB, err := openDB(c.Database.URL)
	if err != nil {
		return err
	}
	defer closeDB()

	return seed.Run(context.Background(), db.DB, os.Stdout, seed.Options{
		Reset:      c.Reset,
		Production: c.Production != "",
		SiteRoot:   c.SiteRoot,
	})
}
