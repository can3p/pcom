package mail

import (
	"fmt"
	"github.com/samber/lo"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/pkg/errors"
)

// PostCommentAuthor formats the notification for the author of a post about
// a new comment, or an edited one when edited is set. It returns nil when
// there is nobody to notify.
func PostCommentAuthor(site links.Site, from string, mediaReplacer types.Replacer[string], user *model.User, author *model.User, post *model.Post, comment *model.PostComment, edited bool) (*Outgoing, error) {
	// we're not sending email notifications to ourselves
	if user.ID == author.ID {
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
	if post.URL != nil {
		urlText = fmt.Sprintf("\nLinked URL: %s", post.URL.URL)
		htmlUrlSection = fmt.Sprintf(`<p>Linked URL: <a href="%s">%s</a></p>`, html.EscapeString(post.URL.URL), html.EscapeString(post.URL.URL))
	}

	verb, subjectLine := "has left a comment in", "New comment in your post"
	uniqueID := comment.ID + user.ID
	if edited {
		verb, subjectLine = "has edited a comment in", "Edited comment in your post"
		// every edit is a new mail, the queue drops a repeated unique id
		uniqueID += lo.FromPtr(comment.EditedAt).UTC().Format(time.RFC3339Nano)
	}

	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: author.Email,
			},
		},
		Subject: fmt.Sprintf("%s \"%s\"", subjectLine, subject),
		Text: fmt.Sprintf(`Hi!

@%s %s your post "%s".%s

%s

Checkout the comment in the post: %s`, user.Username, verb, subject, urlText, "> "+strings.Join(strings.Split(body, "\n"), "\n> "), link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>@%s %s your post "%s".</p>%s

	<blockquote>%s</blockquote>

	<p>Checkout the comment in the <a href="%s">post</a>.</p>`, html.EscapeString(user.Username), verb, html.EscapeString(subject), htmlUrlSection, htmlBody, html.EscapeString(link)),
	}

	return &Outgoing{UniqueID: uniqueID, Type: "comment_notification", Mail: mail}, nil
}
