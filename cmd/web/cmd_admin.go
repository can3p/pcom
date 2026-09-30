package main

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/jessevdk/go-flags"
)

// addAdminCommands registers `admin invite` and `admin registration`.
func addAdminCommands(p *flags.Parser) {
	admin, err := p.AddCommand("admin", "Administer a running site", "Administrative tasks that talk to the database directly.", &struct{}{})
	if err != nil {
		panic(err)
	}

	mustAdd(admin.AddCommand("invite", "Add invites to an account", "Give an account more invites to hand out.", &adminInviteCmd{}))
	mustAdd(admin.AddCommand("registration", "Open or close registration", "Flip the system setting that lets new users sign up.", &adminRegistrationCmd{}))
}

type adminInviteCmd struct {
	config.AdminInvite
}

func (c *adminInviteCmd) Execute([]string) error {
	if c.Num <= 0 {
		return errors.New("--num must be positive")
	}

	db, closeDB, err := openDB(c.Database.URL)
	if err != nil {
		return err
	}
	defer closeDB()

	return registry.New(db, registry.Deps{}).Accounts.AddInvites(context.Background(), c.Email, c.Num)
}

type adminRegistrationCmd struct {
	config.AdminRegistration
}

func (c *adminRegistrationCmd) Execute([]string) error {
	if c.Open == c.Close {
		return errors.New("give exactly one of --open and --close")
	}

	db, closeDB, err := openDB(c.Database.URL)
	if err != nil {
		return err
	}
	defer closeDB()

	return registry.New(db, registry.Deps{}).Accounts.SetRegistrationOpen(context.Background(), c.Open)
}
