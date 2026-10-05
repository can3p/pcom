package main

import (
	"errors"
	"fmt"
	"os"
	_ "time/tzdata" // help go learn about timezones

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/config"
	"github.com/jessevdk/go-flags"
	_ "github.com/joho/godotenv/autoload" // fills the environment from .env before parsing
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			fmt.Println(err)
			return
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses args and executes the chosen command.
func run(args []string) error {
	_, err := settings.Parse(newParser(), args)
	return err
}

// newParser registers every command of the binary.
func newParser() *flags.Parser {
	p := config.NewParser()

	mustAdd(p.AddCommand("serve", "Run the web server", "Serve the site, send queued mail and poll RSS feeds.", &serveCmd{}))
	mustAdd(p.AddCommand("seed", "Fill a development database", "Seed users, posts and comments; refuses to run where FLY_APP_NAME is set.", &seedCmd{}))
	addAdminCommands(p)
	addDebugCommands(p)

	return p
}

func mustAdd(_ *flags.Command, err error) {
	if err != nil {
		panic(err)
	}
}
