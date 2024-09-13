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

func PostPromptAnswer(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, asker, recipient *core.User, post *core.Post, postPrompt *core.PostPrompt) error {
	link := links.AbsLink("post", postPrompt.PostID.String)

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
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

Check out their post! %s`, recipient.Username, postPrompt.Message, post.Subject, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has responded on your prompt "%s" with the post "%s"</p>

	<p>Head to new post page to give an update! <a href="%s">%s</a></p>`, recipient.Username, postPrompt.Message, post.Subject, link, link),
	}

	err := s.Send(ctx, exec, post.ID, "post_prompt_answer", mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}
