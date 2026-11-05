package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// SubscriberManageLinkInput is what the manage link mail shows: the link to
// the page where a subscriber manages their subscriptions.
type SubscriberManageLinkInput struct {
	Site        links.Site
	From        string
	To          string
	ManageToken string
	// RequestID is unique per request, so a repeated request is sent again
	// rather than dropped by the queue as a duplicate.
	RequestID string
}

// Header addresses the mail to the subscriber's address, unique per request.
func (in SubscriberManageLinkInput) Header() Header {
	return Header{Site: in.Site, Audience: AudienceNoAccount, UniqueID: in.RequestID, From: FromPcom(in.From), To: To(in.To)}
}

// Link is the absolute link to the subscriber's manage page.
func (in SubscriberManageLinkInput) Link() string {
	return in.Site.Abs("subscriptions_manage", in.ManageToken)
}

var subscriberManageLinkMail = declare("subscription_manage_link", "subscriber_manage_link", subscriberManageLinkSamples)

func subscriberManageLinkSamples() []Sample[SubscriberManageLinkInput] {
	return []Sample[SubscriberManageLinkInput]{
		{Name: "subscriber_manage_link", Input: SubscriberManageLinkInput{
			Site: SampleSite, From: SampleFrom, To: "reader@example.test",
			ManageToken: "manage-token-1", RequestID: "manage-request-1",
		}},
	}
}

// SubscriberManageLink sends an email subscriber the link to manage their
// subscriptions. requestID identifies this request; the caller passes a value
// unique to it, so repeated requests are each sent.
func SubscriberManageLink(site links.Site, from string, subscriber *model.EmailSubscriber, requestID string) *Envelope {
	return subscriberManageLinkMail.MustRender(SubscriberManageLinkInput{
		Site: site, From: from, To: subscriber.Email,
		ManageToken: subscriber.ManageToken, RequestID: requestID,
	})
}
