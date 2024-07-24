package mail

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func ConfirmSignup(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, user *core.User) error {
	if user.EmailConfirmSeed.String == "" {
		return errors.Errorf("cannot send confirm email for user with empty confirmation seed, user id = %s", user.ID)
	}

	link := links.AbsLink("confirm_signup", user.EmailConfirmSeed.String)
	to := user.Email

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: to,
			},
		},
		Subject: "Welcome to pcom",
		Text: fmt.Sprintf(`
	Hi!

	Thank you for your interest in pcom! Please follow the link to confirm your email address

	%s`, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Thank you for your interest in pcom! Please follow the link to confirm your email address</p>

	<a href="%s">%s</a>`, link, link),
	}

	err := s.Send(ctx, exec, user.ID, "confirm_signup", mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}
