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
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func PostPrompt(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, asker *core.User, recipient *core.User, postPrompt *core.PostPrompt) error {
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

	<p>Head to new post page to give an update! <a href="%s">%s</a></p>`, asker.Username, postPrompt.Message, link, link),
	}

	err := s.Send(ctx, exec, postPrompt.ID, "post_prompt", mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}
