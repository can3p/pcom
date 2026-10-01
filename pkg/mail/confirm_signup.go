package mail

import (
	"fmt"
	"html"
	"net/mail"
	"time"

	"github.com/can3p/gogo/sender"
)

// Envelope is a mail with the id and type the outgoing queue files it under.
// The functions of this package build messages; the services send them.
type Envelope struct {
	UniqueID string
	Type     string
	Mail     *sender.Mail
}

// ConfirmSignup is the mail that carries the code which confirms a new
// account's email address and logs it in, for the login attempt attemptID.
func ConfirmSignup(from string, attemptID, to, code string, lifetime time.Duration) *Envelope {
	minutes := int(lifetime.Minutes())

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

	Thank you for your interest in pcom! Your confirmation code is %s

	Type it on the signup page to confirm your email address and log in. It works once, for the next %d minutes.`, code, minutes),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Thank you for your interest in pcom! Your confirmation code is <strong>%s</strong></p>

	<p>Type it on the signup page to confirm your email address and log in. It works once, for the next %d minutes.</p>`, html.EscapeString(code), minutes),
	}

	return &Envelope{UniqueID: attemptID, Type: "confirm_signup", Mail: mail}
}
