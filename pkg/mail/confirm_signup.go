package mail

import (
	"fmt"
	"html"
	"net/mail"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/pkg/errors"
)

// Envelope is a mail with the id and type the outgoing queue files it under.
// The functions of this package build messages; the services send them.
type Envelope struct {
	UniqueID string
	Type     string
	Mail     *sender.Mail
}

// ConfirmSignup is the mail with the link that confirms a new account's
// email address.
func ConfirmSignup(site links.Site, from string, user *core.User) (*Envelope, error) {
	if user.EmailConfirmSeed.String == "" {
		return nil, errors.Errorf("cannot send confirm email for user with empty confirmation seed, user id = %s", user.ID)
	}

	link := site.Abs("confirm_signup", user.EmailConfirmSeed.String)
	to := user.Email

	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
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

	<a href="%s">%s</a>`, html.EscapeString(link), html.EscapeString(link)),
	}

	return &Envelope{UniqueID: user.ID, Type: "confirm_signup", Mail: mail}, nil
}
