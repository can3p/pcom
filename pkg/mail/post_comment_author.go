package mail

import (
	"fmt"
	"html"
	"net/mail"
	"os"
	"strings"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/pkg/errors"
)

// PostCommentAuthor formats the notification for the author of a post about
// a new comment. It returns nil when there is nobody to notify.
func PostCommentAuthor(mediaReplacer types.Replacer[string], user *core.User, author *core.User, post *core.Post, comment *core.PostComment) (*Outgoing, error) {
	// we're not sending email notifications to ourselves
	if user.ID == author.ID {
		return nil, nil
	}

	link := links.AbsLink("comment", post.ID, comment.ID)
	body, err := markdown.ReplaceImageUrls(comment.Body, mediaReplacer)
	if err != nil {
		// ReplaceImageUrls only fails if goldmark cannot render, which no input triggers, so no test covers this.
		return nil, errors.Wrap(err, "failed to render the comment")
	}
	htmlBody := markdown.ToEnrichedTemplate(comment.Body, types.ViewEmail, mediaReplacer, links.AbsLink)

	subject := postops.PostSubject(post.Subject)

	// Get linked URL if available
	var urlText string
	var htmlUrlSection string
	if post.R != nil && post.R.URL != nil {
		urlText = fmt.Sprintf("\nLinked URL: %s", post.R.URL.URL)
		htmlUrlSection = fmt.Sprintf(`<p>Linked URL: <a href="%s">%s</a></p>`, html.EscapeString(post.R.URL.URL), html.EscapeString(post.R.URL.URL))
	}

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: author.Email,
			},
		},
		Subject: fmt.Sprintf("New comment in your post \"%s\"", subject),
		Text: fmt.Sprintf(`Hi!

@%s has left a comment in your post "%s".%s

%s

Checkout the comment in the post: %s`, user.Username, subject, urlText, "> "+strings.Join(strings.Split(body, "\n"), "\n> "), link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has left a comment in your post "%s".</p>%s

	<blockquote>%s</blockquote>

	<p>Checkout the comment in the <a href="%s">post</a>.</p>`, html.EscapeString(user.Username), html.EscapeString(subject), htmlUrlSection, htmlBody, html.EscapeString(link)),
	}

	return &Outgoing{UniqueID: comment.ID + user.ID, Type: "comment_notification", Mail: mail}, nil
}
