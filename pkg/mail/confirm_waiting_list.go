package mail

import (
	"fmt"
	"html"
	"net/mail"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
)

// ConfirmWaitingList is the mail with the link that confirms a waiting list
// entry's email address.
func ConfirmWaitingList(site links.Site, from string, waitingList *core.UserSignupRequest) *Envelope {
	link := site.Abs("confirm_waiting_list", waitingList.ID)
	to := waitingList.Email

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
		Subject: "Waiting list on pcom",
		Text: fmt.Sprintf(`
	Hi!

	Thank you for your interest pcom! Please follow the link to confirm your email address

	%s`, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Thank you for your interest pcom! Please follow the link to confirm your email address</p>

	<a href="%s">%s</a>`, html.EscapeString(link), html.EscapeString(link)),
	}

	return &Envelope{UniqueID: waitingList.ID, Type: "waiting_list_confirm", Mail: mail}
}
