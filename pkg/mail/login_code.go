package mail

import (
	"fmt"
	"html"
	"net/mail"
	"time"

	"github.com/can3p/gogo/sender"
)

// LoginCode is the mail that carries the code for one login attempt to the
// address it was started with.
func LoginCode(from string, attemptID, to, code string, lifetime time.Duration) *Envelope {
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
		Subject: "Your pcom login code",
		Text: fmt.Sprintf(`
	Hi!

	Your pcom login code is %s

	It works once, for the next %d minutes.

	If you didn't try to log in to pcom, ignore this mail: nobody can log in without the code.`, code, minutes),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Your pcom login code is <strong>%s</strong></p>

	<p>It works once, for the next %d minutes.</p>

	<p>If you didn't try to log in to pcom, ignore this mail: nobody can log in without the code.</p>`, html.EscapeString(code), minutes),
	}

	return &Envelope{UniqueID: attemptID, Type: "login_code", Mail: mail}
}
