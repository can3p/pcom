package mail

import (
	"fmt"
	"html"
	"net/mail"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
)

// Invitation is the mail that carries an invitation link to the address it
// was sent to.
func Invitation(site links.Site, from string, invite *core.UserInvitation, to string) *Envelope {
	link := site.Abs("invite", invite.ID)

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

	Welcome to pcom! Please follow the link to choose a username. You will log in with a code we mail you, so there is no password to set.

	%s`, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Welcome to pcom! Please follow the link to choose a username. You will log in with a code we mail you, so there is no password to set.</p>

	<a href="%s">%s</a>`, html.EscapeString(link), html.EscapeString(link)),
	}

	return &Envelope{UniqueID: invite.ID, Type: "user_invitation", Mail: mail}
}
