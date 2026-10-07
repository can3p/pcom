package mail

import (
	htmltemplate "html/template"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
)

// NewPostInput is what the new post mail shows: the author, the post and
// the connection it is sent to.
type NewPostInput struct {
	From         string
	PostID       string
	ConnectionID string
	To           string
	Username     string
	Subject      string
	// LinkedURL is the url the post links to, empty when it has none.
	LinkedURL string
	Link      string
	// Body is the post body rendered by markdown.ToEnrichedTemplate.
	Body htmltemplate.HTML
}

// Header addresses the mail to the connection, unique per post and
// connection.
func (in NewPostInput) Header() Header {
	return Header{UniqueID: in.PostID + in.ConnectionID, From: FromPcom(in.From), To: To(in.To)}
}

var newPostMail = declare("post_notification", "new_post", newPostSamples)

// newPostBody renders the markdown body of a post for the mail.
func newPostBody(site links.Site, postID, body string, mediaReplacer types.Replacer[string]) htmltemplate.HTML {
	return markdown.ToEnrichedTemplate(body, types.ViewEmail, mediaReplacer, func(in string, add2 ...string) string {
		if in == "single_post_special" {
			args := []string{postID}
			args = append(args, add2...)

			return site.Abs("post", args...)
		}

		return site.Abs(in, add2...)
	})
}

func newPostSamples() []Sample[NewPostInput] {
	keep := func(in string) (bool, string) { return false, in }

	sample := func(subject, linkedURL, body string) NewPostInput {
		return NewPostInput{
			From: SampleFrom, PostID: "post-1", ConnectionID: "user-2", To: "connection@example.test",
			Username: "alice", Subject: subject, LinkedURL: linkedURL,
			Link: SampleSite.Abs("post", "post-1"),
			Body: newPostBody(SampleSite, "post-1", body, keep),
		}
	}

	const body = "This is a test post body"

	return []Sample[NewPostInput]{
		{Name: "new_post", Input: sample("Test Post", "", body)},
		{Name: "new_post_no_subject", Input: sample(postops.PostSubject(nil), "", body)},
		{Name: "new_post_with_url", Input: sample("Check out this article", "https://example.com/article", body)},
		{Name: "new_post_special_chars", Input: sample(`Test <b>bold</b> and "quotes"`, "", body)},
		{Name: "new_post_empty_body", Input: sample("Test Post", "", "")},
		{Name: "new_post_long_subject", Input: sample("A very long subject that goes on and on and on to see how the mail copes with a title that no one would reasonably write but someone surely will", "", body)},
		{Name: "new_post_markup_in_body", Input: sample("Test Post", "", `Hello <script>alert(1)</script> and <a href="https://example.com/x?a=1&b=2">a link</a>`)},
	}
}

// NewPost formats the notification about a new post for one connection of its
// author. It returns nil when there is nobody to notify.
func NewPost(site links.Site, from string, mediaReplacer types.Replacer[string], user *model.User, connection *model.User, post *model.Post) *Envelope {
	// we're not sending email notifications to ourselves
	if user.ID == connection.ID {
		return nil
	}

	in := NewPostInput{
		From:         from,
		PostID:       post.ID,
		ConnectionID: connection.ID,
		To:           connection.Email,
		Username:     user.Username,
		Subject:      postops.PostSubject(post.Subject),
		Link:         site.Abs("post", post.ID),
		// there reason to omit body in the text version is that we should redo the logic with cut, gallery etc
		// and I have no desire to spend time on that
		Body: newPostBody(site, post.ID, post.Body, mediaReplacer),
	}

	if post.URL != nil {
		in.LinkedURL = post.URL.URL
	}

	return newPostMail.MustRender(in)
}
