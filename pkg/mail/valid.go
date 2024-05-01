package mail

import (
	"context"

	"github.com/badoux/checkmail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func Validate(ctx context.Context, db boil.ContextExecutor, email string) error {
	if err := checkmail.ValidateFormat(email); err != nil {
		return errors.Errorf("Invalid email format")
	}

	if core.Users(
		core.UserWhere.Email.EQ(email),
	).ExistsP(ctx, db) {
		return errors.Errorf("email is already registered in the system")
	}

	return nil
}
