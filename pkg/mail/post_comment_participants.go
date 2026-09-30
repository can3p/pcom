package mail

import (
	"fmt"
	"html"
	"net/mail"
	"strings"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/pkg/errors"
)

// PostCommentParticipants formats the notification about a new comment for a
// user who commented on the post earlier. It returns nil when there is nobody
// to notify.
func PostCommentParticipants(site links.Site, from string, mediaReplacer types.Replacer[string], commentAuthor *core.User, participant *core.User, post *core.Post, comment *core.PostComment) (*Outgoing, error) {
	// we're not sending email notifications to ourselves
	if commentAuthor.ID == participant.ID {
		return nil, nil
	}

	link := site.Abs("comment", post.ID, comment.ID)
	body, err := markdown.ReplaceImageUrls(comment.Body, mediaReplacer)
	if err != nil {
		// ReplaceImageUrls only fails if goldmark cannot render, which no input triggers, so no test covers this.
		return nil, errors.Wrap(err, "failed to render the comment")
	}
	htmlBody := markdown.ToEnrichedTemplate(comment.Body, types.ViewEmail, mediaReplacer, site.Abs)

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
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: participant.Email,
			},
		},
		Subject: fmt.Sprintf("New comment in the post \"%s\"", subject),
		Text: fmt.Sprintf(`Hi!

@%s has left a comment in the post "%s" where you've also left a comment.%s

%s

Checkout the comment in the post: %s`, commentAuthor.Username, subject, urlText, "> "+strings.Join(strings.Split(body, "\n"), "\n> "), link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s has left a comment in the post "%s" where you've also left a comment.</p>%s

	<blockquote>%s</blockquote>

	<p>Checkout the comment in the <a href="%s">post</a>.</p>`, html.EscapeString(commentAuthor.Username), html.EscapeString(subject), htmlUrlSection, htmlBody, html.EscapeString(link)),
	}

	return &Outgoing{UniqueID: comment.ID + participant.ID, Type: "comment_notification", Mail: mail}, nil
}
