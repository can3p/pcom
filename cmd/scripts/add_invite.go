package main

import (
	"context"
	"database/sql"
	"flag"
	"os"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // postgres db driver
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func main() {
	db := sqlx.MustConnect("postgres", os.Getenv("DATABASE_URL")+"?sslmode=disable")
	defer db.Close()

	email := flag.String("email", "", "account email")
	num := flag.Int("num", 0, "number of invites")

	flag.Parse()

	if *email == "" {
		panic("email is required")
	}

	u := core.Users(
		core.UserWhere.Email.EQ(*email),
	).OneP(context.Background(), db)

	err := transact.Transact(db, func(tx *sql.Tx) error {
		for *num > 0 {
			inv := core.UserInvitation{
				ID:     uuid.NewString(),
				UserID: u.ID,
			}

			inv.InsertP(context.Background(), tx, boil.Infer())
			*num--
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}
