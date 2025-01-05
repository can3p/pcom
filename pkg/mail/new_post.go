package mail

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func NewPost(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, mediaReplacer types.Replacer[string], user *core.User, connection *core.User, post *core.Post) error {
	// we're not sending email notifications to ourselves
	if user.ID == connection.ID {
		return nil
	}

	link := links.AbsLink("post", post.ID)
	// there reason to omit body in the text version is that we should redo the logic with cut, gallery etc
	// and I have no desire to spend time on that
	htmlbody := markdown.ToEnrichedTemplate(post.Body, types.ViewEmail, mediaReplacer, func(in string, add2 ...string) string {
		if in == "single_post_special" {
			args := []string{post.ID}
			args = append(args, add2...)

			return links.AbsLink("post", args...)
		}

		return links.AbsLink(in, add2...)
	})

	subject := postops.PostSubject(post.Subject)

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: connection.Email,
			},
		},
		Subject: fmt.Sprintf("New post \"%s\" from %s", subject, user.Username),
		Text: fmt.Sprintf(`Hi!

@%s has published a new post "%s"

Head to the post to leave a comment! %s`, user.Username, subject, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has published a new post "%s"</p>

	<blockquote>%s</blockquote>

	<p>Head to the post to leave a comment! <a href="%s">%s</a></p>`, user.Username, subject, htmlbody, link, link),
	}

	err := s.Send(ctx, exec, post.ID+connection.ID, "post_notification", mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}
