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

// postCommentSamples are the samples both comment mails share, built from
// in-memory models. name is the sample name of the plain one, the others add
// a suffix; to is the recipient's address and body the comment of the plain and
// the linked-url ones.
func postCommentSamples(name, to, body string) []postCommentSample {
	identity := func(in string) (bool, string) { return false, in }
	commenter := &model.User{ID: "user-1", Email: "commenter@example.test", Username: "alice"}
	recipient := &model.User{ID: "user-2", Email: to, Username: "bob"}
	post := func(subject string) *model.Post {
		return &model.Post{ID: "post-1", Subject: &subject, Body: "Post body", UserID: "user-3"}
	}
	comment := func(body string) *model.PostComment {
		return &model.PostComment{ID: "comment-1", PostID: "post-1", UserID: commenter.ID, Body: body}
	}
	editedRecipient := &model.User{ID: "user-2", Email: "recipient@example.test", Username: "bob"}
	build := func(p *model.Post, c *model.PostComment, edited bool) postComment {
		to := recipient
		if edited {
			to = editedRecipient
		}

		in, err := newPostComment(SampleSite, SampleFrom, identity, commenter, to, p, c, edited)
		if err != nil {
			panic(err)
		}

		return in
	}

	withURL := post("Original Post")
	withURL.URL = &model.NormalizedURL{ID: "url-1", URL: "https://example.com/article"}
	edited := comment("Nice post, edited!")
	edited.EditedAt = new(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))

	return []postCommentSample{
		{name, build(post("Original Post"), comment(body), false)},
		{name + "_edited", build(post("Original Post"), edited, true)},
		{name + "_with_url", build(withURL, comment(body), false)},
		{name + "_markup", build(post("Original Post"), comment("Nice **post**, see [the docs](https://example.com/a?b=1&c=2).\n\n- one\n- <script>alert(1)</script>\n\n> quoted & \"done\""), false)},
		{name + "_markup_long_subject", build(post("A <b>very</b> long & \"quoted\" subject that goes on and on to see how a mail client wraps a subject line of this length, past any sensible limit"), comment("Nice **post**!"), false)},
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
