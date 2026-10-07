package mail

import (
	"html/template"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// postComment is what the two comment mails show: one comment, the post it
// belongs to and who it is for. The mails differ in their wording only.
type postComment struct {
	From         string
	To           string
	UniqueUserID string
	CommentID    string
	Commenter    string
	Subject      string
	// URL is the url the post links to, empty when it links to none.
	URL string
	// Link is the absolute link to the comment.
	Link string
	// Body is the comment as markdown with its image urls replaced.
	Body string
	// BodyHTML is the comment rendered for mail.
	BodyHTML template.HTML
	Edited   bool
	EditedAt time.Time
}

// uniqueID is the comment and the recipient; every edit is a new mail, the
// queue drops a repeated unique id.
func (in postComment) uniqueID() string {
	id := in.CommentID + in.UniqueUserID
	if in.Edited {
		id += in.EditedAt.UTC().Format(time.RFC3339Nano)
	}

	return id
}

func (in postComment) header() Header {
	return Header{UniqueID: in.uniqueID(), From: FromPcom(in.From), To: To(in.To)}
}

// Quote is the comment as a text quote.
func (in postComment) Quote() string {
	return "> " + strings.Join(strings.Split(in.Body, "\n"), "\n> ")
}

// newPostComment maps the models of a comment mail to its input.
func newPostComment(site links.Site, from string, mediaReplacer types.Replacer[string], commenter, recipient *model.User, post *model.Post, comment *model.PostComment, edited bool) (postComment, error) {
	body, err := markdown.ReplaceImageUrls(comment.Body, mediaReplacer)
	if err != nil {
		// ReplaceImageUrls only fails if goldmark cannot render, which no input triggers, so no test covers this.
		return postComment{}, errors.Wrap(err, "failed to render the comment")
	}

	in := postComment{
		From:         from,
		To:           recipient.Email,
		UniqueUserID: recipient.ID,
		CommentID:    comment.ID,
		Commenter:    commenter.Username,
		Subject:      postops.PostSubject(post.Subject),
		Link:         site.Abs("comment", post.ID, comment.ID),
		Body:         body,
		BodyHTML:     markdown.ToEnrichedTemplate(comment.Body, types.ViewEmail, mediaReplacer, site.Abs),
		Edited:       edited,
		EditedAt:     lo.FromPtr(comment.EditedAt),
	}
	if post.URL != nil {
		in.URL = post.URL.URL
	}

	return in, nil
}

type postCommentSample struct {
	name string
	in   postComment
}

// postCommentSamples are the samples both comment mails share, written as
// literal inputs so that no mapper from models stands on both sides of the
// tests. name is the sample name of the plain one, the others add a suffix;
// to is the recipient's address and body the comment of the plain and the
// linked-url ones.
func postCommentSamples(name, to, body string) []postCommentSample {
	const (
		commentLink = "https://pcom.test/posts/post-1#commentpost-1comment-1"
		subject     = "Original Post"
	)

	return []postCommentSample{
		{name, postComment{
			From: SampleFrom, To: to, UniqueUserID: "user-2", CommentID: "comment-1", Commenter: "alice",
			Subject: subject, URL: "", Link: commentLink, Body: body, BodyHTML: template.HTML("<p>" + body + "</p>\n"),
			Edited: false, EditedAt: time.Time{},
		}},
		{name + "_edited", postComment{
			From: SampleFrom, To: "recipient@example.test", UniqueUserID: "user-2", CommentID: "comment-1", Commenter: "alice",
			Subject: subject, URL: "", Link: commentLink, Body: "Nice post, edited!", BodyHTML: "<p>Nice post, edited!</p>\n",
			Edited: true, EditedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
		}},
		{name + "_with_url", postComment{
			From: SampleFrom, To: to, UniqueUserID: "user-2", CommentID: "comment-1", Commenter: "alice",
			Subject: subject, URL: "https://example.com/article", Link: commentLink, Body: body, BodyHTML: template.HTML("<p>" + body + "</p>\n"),
			Edited: false, EditedAt: time.Time{},
		}},
		{name + "_markup", postComment{
			From: SampleFrom, To: to, UniqueUserID: "user-2", CommentID: "comment-1", Commenter: "alice",
			Subject: subject, URL: "", Link: commentLink,
			Body:     "Nice **post**, see [the docs](https://example.com/a?b=1&c=2).\n\n- one\n- <script>alert(1)</script>\n\n> quoted & \"done\"",
			BodyHTML: "<p>Nice <strong>post</strong>, see <a href=\"https://example.com/a?b=1&amp;c=2\">the docs</a>.</p>\n<ul>\n<li>one</li>\n<li>\n<!-- raw HTML omitted -->\n</li>\n</ul>\n<blockquote>\n<p>quoted &amp; &quot;done&quot;</p>\n</blockquote>\n",
			Edited:   false, EditedAt: time.Time{},
		}},
		{name + "_markup_long_subject", postComment{
			From: SampleFrom, To: to, UniqueUserID: "user-2", CommentID: "comment-1", Commenter: "alice",
			Subject: "A <b>very</b> long & \"quoted\" subject that goes on and on to see how a mail client wraps a subject line of this length, past any sensible limit",
			URL:     "", Link: commentLink, Body: "Nice **post**!", BodyHTML: "<p>Nice <strong>post</strong>!</p>\n",
			Edited: false, EditedAt: time.Time{},
		}},
	}
}

// PostCommentAuthorInput is what the mail to the author of a post about a
// comment shows.
type PostCommentAuthorInput struct{ postComment }

// Header addresses the mail to the author of the post.
func (in PostCommentAuthorInput) Header() Header { return in.header() }

var postCommentAuthorMail = declare("comment_notification", "post_comment_author", postCommentAuthorSamples)

func postCommentAuthorSamples() []Sample[PostCommentAuthorInput] {
	return lo.Map(postCommentSamples("post_comment_author", "author@example.test", "Nice post!"), func(s postCommentSample, _ int) Sample[PostCommentAuthorInput] {
		s.in.UniqueUserID = "user-1" // the commenter, as in PostCommentAuthor
		return Sample[PostCommentAuthorInput]{Name: s.name, Input: PostCommentAuthorInput{s.in}}
	})
}

// PostCommentAuthor formats the notification for the author of a post about
// a new comment, or an edited one when edited is set. It returns nil when
// there is nobody to notify.
func PostCommentAuthor(site links.Site, from string, mediaReplacer types.Replacer[string], user *model.User, author *model.User, post *model.Post, comment *model.PostComment, edited bool) (*Envelope, error) {
	// we're not sending email notifications to ourselves
	if user.ID == author.ID {
		return nil, nil
	}

	in, err := newPostComment(site, from, mediaReplacer, user, author, post, comment, edited)
	if err != nil {
		return nil, err
	}

	// the unique id of this mail has always been made of the commenter, not of the recipient
	in.UniqueUserID = user.ID

	return postCommentAuthorMail.Render(PostCommentAuthorInput{in})
}
