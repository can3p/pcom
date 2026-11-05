package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// AuthorSubscriberLeftInput is what the subscriber left mail to an author
// shows: the address that unsubscribed and the link to the author's controls.
type AuthorSubscriberLeftInput struct {
	Site            links.Site
	From            string
	To              string
	SubscriptionID  string
	SubscriberEmail string
}

// Header addresses the mail to the author, unique per subscription.
func (in AuthorSubscriberLeftInput) Header() Header {
	return Header{Site: in.Site, Audience: AudienceAccount, UniqueID: in.SubscriptionID, From: FromPcom(in.From), To: To(in.To)}
}

// Link is the absolute link to the author's controls.
func (in AuthorSubscriberLeftInput) Link() string {
	return in.Site.Abs("controls")
}

var authorSubscriberLeftMail = declare("subscription_author_left", "author_subscriber_left", authorSubscriberLeftSamples)

func authorSubscriberLeftSamples() []Sample[AuthorSubscriberLeftInput] {
	return []Sample[AuthorSubscriberLeftInput]{
		{Name: "author_subscriber_left", Input: AuthorSubscriberLeftInput{
			Site: SampleSite, From: SampleFrom, To: "alice@example.test",
			SubscriptionID: "subscription-1", SubscriberEmail: "reader@example.test",
		}},
	}
}

// AuthorSubscriberLeft tells an author that an address unsubscribed from
// their public posts. It is queued in the transaction that deletes the
// subscription.
func AuthorSubscriberLeft(site links.Site, from string, author *model.User, subscriber *model.EmailSubscriber, subscription *model.EmailSubscription) *Envelope {
	return authorSubscriberLeftMail.MustRender(AuthorSubscriberLeftInput{
		Site: site, From: from, To: author.Email,
		SubscriptionID: subscription.ID, SubscriberEmail: subscriber.Email,
	})
}
