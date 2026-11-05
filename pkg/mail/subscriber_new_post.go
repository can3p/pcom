package mail

import (
	htmltemplate "html/template"
	"net/mail"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/types"
)

// SubscriberNewPostInput is what the new post mail to an email subscriber
// shows: the author's post and the links that manage and end the subscription.
type SubscriberNewPostInput struct {
	Site           links.Site
	From           string
	PostID         string
	SubscriptionID string
	To             string
	AuthorUsername string
	ManageToken    string
	Subject        string
	// LinkedURL is the url the post links to, empty when it has none.
	LinkedURL string
	// Body is the post body rendered by markdown.ToEnrichedTemplate.
	Body htmltemplate.HTML
}

// Header addresses the mail to the subscriber, from the author, unique per
// post and subscription. It carries the footer's links and the RFC 8058
// one-click unsubscribe headers.
func (in SubscriberNewPostInput) Header() Header {
	return Header{
		Site:           in.Site,
		Audience:       AudienceSubscriber,
		AuthorUsername: in.AuthorUsername,
		UniqueID:       in.PostID + in.SubscriptionID,
		From:           mail.Address{Address: in.From, Name: "@" + in.AuthorUsername + " on pcom"},
		To:             To(in.To),
		ManageURL:      in.ManageLink(),
		UnsubscribeURL: in.UnsubscribeLink(),
	}
}

// Link is the absolute link to the post on pcom.
func (in SubscriberNewPostInput) Link() string {
	return in.Site.Abs("post", in.PostID)
}

// ManageLink is the absolute link where the subscriber manages subscriptions.
func (in SubscriberNewPostInput) ManageLink() string {
	return in.Site.Abs("subscriptions_manage", in.ManageToken)
}

// UnsubscribeLink is the absolute one-click unsubscribe link of the
// subscription.
func (in SubscriberNewPostInput) UnsubscribeLink() string {
	return in.Site.Abs("subscription_unsubscribe", in.ManageToken, in.SubscriptionID)
}

var subscriberNewPostMail = declare("subscription_post", "subscriber_new_post", subscriberNewPostSamples)

func subscriberNewPostSamples() []Sample[SubscriberNewPostInput] {
	keep := func(in string) (bool, string) { return false, in }

	sample := func(subject, linkedURL, body string) SubscriberNewPostInput {
		return SubscriberNewPostInput{
			Site: SampleSite, From: SampleFrom, PostID: "post-1", SubscriptionID: "subscription-1",
			To: "subscriber@example.test", AuthorUsername: "alice", ManageToken: "manage-token-1",
			Subject: subject, LinkedURL: linkedURL,
			Body: newPostBody(SampleSite, "post-1", body, keep),
		}
	}

	const body = "This is a test post body"

	const prose = "Ivan walked, and his brain was boiling. Around him was a wet and cold autumn, the sky was not to be seen.\n\n" +
		"Did he think about something? Was he replaying the events of the day before?\n\n{cut}\n\nText after the cut."

	return []Sample[SubscriberNewPostInput]{
		{Name: "subscriber_new_post", Input: sample("Test Post", "", body)},
		{Name: "subscriber_new_post_cut", Input: sample("Some day", "", prose)},
		{Name: "subscriber_new_post_no_subject", Input: sample(postops.PostSubject(nil), "", body)},
		{Name: "subscriber_new_post_with_url", Input: sample("Check out this article", "https://example.com/article", body)},
	}
}

// SubscriberNewPost tells an email subscriber of the author about a new post.
func SubscriberNewPost(site links.Site, from string, mediaReplacer types.Replacer[string], author *model.User, subscriber *model.EmailSubscriber, subscription *model.EmailSubscription, post *model.Post) *Envelope {
	in := SubscriberNewPostInput{
		Site: site, From: from, PostID: post.ID, SubscriptionID: subscription.ID,
		To: subscriber.Email, AuthorUsername: author.Username, ManageToken: subscriber.ManageToken,
		Subject: postops.PostSubject(post.Subject),
		Body:    newPostBody(site, post.ID, post.Body, mediaReplacer),
	}

	if post.URL != nil {
		in.LinkedURL = post.URL.URL
	}

	return subscriberNewPostMail.MustRender(in)
}
