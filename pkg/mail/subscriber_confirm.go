package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// SubscriberConfirmInput is what the subscription confirmation mail shows: the
// author the reader subscribes to, and the link that confirms the
// subscription.
type SubscriberConfirmInput struct {
	Site           links.Site
	From           string
	To             string
	AuthorUsername string
	SubscriptionID string
	ConfirmToken   string
}

// Header addresses the mail to the subscriber's address, unique per
// subscription and confirmation token.
func (in SubscriberConfirmInput) Header() Header {
	return Header{Site: in.Site, Audience: AudienceNoAccount, UniqueID: in.SubscriptionID + in.ConfirmToken, From: FromPcom(in.From), To: To(in.To)}
}

// Link is the absolute link that confirms the subscription.
func (in SubscriberConfirmInput) Link() string {
	return in.Site.Abs("subscription_confirm", in.ConfirmToken)
}

var subscriberConfirmMail = declare("subscription_confirm", "subscriber_confirm", subscriberConfirmSamples)

func subscriberConfirmSamples() []Sample[SubscriberConfirmInput] {
	return []Sample[SubscriberConfirmInput]{
		{Name: "subscriber_confirm", Input: SubscriberConfirmInput{
			Site: SampleSite, From: SampleFrom, To: "reader@example.test",
			AuthorUsername: "alice", SubscriptionID: "subscription-1", ConfirmToken: "confirm-token-1",
		}},
	}
}

// SubscriberConfirm asks a reader to confirm a subscription to an author's
// public posts. The author will see the reader's address.
func SubscriberConfirm(site links.Site, from string, author *model.User, subscriber *model.EmailSubscriber, subscription *model.EmailSubscription) *Envelope {
	return subscriberConfirmMail.MustRender(SubscriberConfirmInput{
		Site: site, From: from, To: subscriber.Email, AuthorUsername: author.Username,
		SubscriptionID: subscription.ID, ConfirmToken: subscription.ConfirmToken,
	})
}
