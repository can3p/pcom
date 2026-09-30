package mail

import (
	"fmt"
	"html"
	"net/mail"
	"os"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
)

// PostPrompt formats the notification about a prompt for its recipient. It
// returns nil when there is nobody to notify.
func PostPrompt(asker *core.User, recipient *core.User, postPrompt *core.PostPrompt) *Outgoing {
	// we're not sending email notifications to ourselves
	if asker.ID == recipient.ID {
		return nil
	}

	link := links.AbsLink("write", "prompt", postPrompt.ID)

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: recipient.Email,
			},
		},
		Subject: fmt.Sprintf("New prompt from %s", asker.Username),
		Text: fmt.Sprintf(`Hi!

@%s has asked you to write a post on "%s"

Head to new post page to give an update! %s`, asker.Username, postPrompt.Message, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has asked you to write a post on "%s"</p>

	<p>Head to new post page to give an update! <a href="%s">%s</a></p>`, html.EscapeString(asker.Username), html.EscapeString(postPrompt.Message), html.EscapeString(link), html.EscapeString(link)),
	}

	return &Outgoing{UniqueID: postPrompt.ID, Type: "post_prompt", Mail: mail}
}
