package mail

import (
	"fmt"
	"html"
	"net/mail"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
)

// PostPromptAnswer formats the notification for the asker of a prompt that
// the recipient answered with a post.
func PostPromptAnswer(site links.Site, from string, asker, recipient *core.User, post *core.Post, postPrompt *core.PostPrompt) *Outgoing {
	link := site.Abs("post", postPrompt.PostID.String)

	subject := postops.PostSubject(post.Subject)

	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: asker.Email,
			},
		},
		Subject: fmt.Sprintf("Response to your prompt from %s", recipient.Username),
		Text: fmt.Sprintf(`Hi!

@%s has responded on your prompt "%s" with the post "%s"

Check out their post! %s`, recipient.Username, postPrompt.Message, subject, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has responded on your prompt "%s" with the post "%s"</p>

	<p>Head to new post page to give an update! <a href="%s">%s</a></p>`, html.EscapeString(recipient.Username), html.EscapeString(postPrompt.Message), html.EscapeString(subject), html.EscapeString(link), html.EscapeString(link)),
	}

	return &Outgoing{UniqueID: post.ID, Type: "post_prompt_answer", Mail: mail}
}
