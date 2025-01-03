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
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/types"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func PostCommentParticipants(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, mediaReplacer types.Replacer[string], commentAuthor *core.User, participant *core.User, post *core.Post, comment *core.PostComment) error {
	// we're not sending email notifications to ourselves
	if commentAuthor.ID == participant.ID {
		return nil
	}

	link := links.AbsLink("comment", post.ID, comment.ID)
	body := markdown.ReplaceImageUrls(comment.Body, mediaReplacer)
	htmlBody := markdown.ToEnrichedTemplate(comment.Body, types.ViewEmail, mediaReplacer, links.AbsLink)

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

@%s has left a comment in the post "%s" where you've also left a comment.

%s

Checkout the comment in the post: %s`, commentAuthor.Username, post.Subject, "> "+strings.Join(strings.Split(body, "\n"), "\n> "), link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has left a comment in the post "%s" where you've also left a comment.</p>

	<blockquote>%s</blockquote>

	<p>Checkout the comment in the <a href="%s">post</a>.</p>`, commentAuthor.Username, post.Subject, htmlBody, link),
	}

	err := s.Send(ctx, exec, comment.ID+participant.ID, "comment_notification", mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}
