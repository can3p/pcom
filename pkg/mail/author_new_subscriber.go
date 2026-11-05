package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// AuthorNewSubscriberInput is what the new subscriber mail to an author shows:
// the address that subscribed and the link to the author's controls.
type AuthorNewSubscriberInput struct {
	Site            links.Site
	From            string
	To              string
	SubscriptionID  string
	SubscriberEmail string
}

// Header addresses the mail to the author, unique per subscription.
func (in AuthorNewSubscriberInput) Header() Header {
	return Header{Site: in.Site, Audience: AudienceAccount, UniqueID: in.SubscriptionID, From: FromPcom(in.From), To: To(in.To)}
}

// Link is the absolute link to the author's controls.
func (in AuthorNewSubscriberInput) Link() string {
	return in.Site.Abs("controls")
}

var authorNewSubscriberMail = declare("subscription_author_new", "author_new_subscriber", authorNewSubscriberSamples)

func authorNewSubscriberSamples() []Sample[AuthorNewSubscriberInput] {
	return []Sample[AuthorNewSubscriberInput]{
		{Name: "author_new_subscriber", Input: AuthorNewSubscriberInput{
			Site: SampleSite, From: SampleFrom, To: "alice@example.test",
			SubscriptionID: "subscription-1", SubscriberEmail: "reader@example.test",
		}},
	}
}

// AuthorNewSubscriber tells an author that an address subscribed to their
// public posts.
func AuthorNewSubscriber(site links.Site, from string, author *model.User, subscriber *model.EmailSubscriber, subscription *model.EmailSubscription) *Envelope {
	return authorNewSubscriberMail.MustRender(AuthorNewSubscriberInput{
		Site: site, From: from, To: author.Email,
		SubscriptionID: subscription.ID, SubscriberEmail: subscriber.Email,
	})
}
