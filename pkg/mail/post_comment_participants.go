package mail

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
)

func PostCommentParticipants(ctx context.Context, s sender.Sender, user *core.User, participant *core.User, post *core.Post, comment *core.PostComment) error {
	// we're not sending email notifications to ourselves
	if user.ID == participant.ID {
		return nil
	}

	if post.UserID == user.ID {
		return nil
	}

	link := links.AbsLink("post", post.ID)

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: participant.Email,
			},
		},
		Subject: fmt.Sprintf("New comment in the post \"%s\"", post.Subject),
		Text: fmt.Sprintf(`Hi!

@%s has left a comment in your post "%s" where you've also left a comment.

%s

Checkout the post: %s`, user.Username, post.Subject, "> "+strings.Join(strings.Split(comment.Body, "\n"), "\n> "), link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has left a comment in your post "%s" where you've also left a comment.</p>

	<blockquote>%s</blockquote>

	<p>Checkout the post: <a href="%s">%s</a></p>`, user.Username, post.Subject, comment.Body, link, link),
	}

	err := s.Send(ctx, mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}