package mail

import (
	"time"

	"github.com/can3p/pcom/pkg/links"
)

// AdminSubscriptionsBrakedInput is what the alert to the admin shows when
// email subscriptions are braked: the hour the brake was raised.
type AdminSubscriptionsBrakedInput struct {
	Site         links.Site
	From         string
	AdminAddress string
	// Hour is the UTC hour of the alert, RFC 3339, which is also its unique id.
	Hour string
}

// Header addresses the mail to the admin, unique per hour.
func (in AdminSubscriptionsBrakedInput) Header() Header {
	return Header{Site: in.Site, Audience: AudienceAccount, UniqueID: in.Hour, From: FromPcom(in.From), To: To(in.AdminAddress)}
}

var adminSubscriptionsBrakedMail = declare("admin_subscriptions_braked", "admin_subscriptions_braked", adminSubscriptionsBrakedSamples)

func adminSubscriptionsBrakedSamples() []Sample[AdminSubscriptionsBrakedInput] {
	return []Sample[AdminSubscriptionsBrakedInput]{
		{Name: "admin_subscriptions_braked", Input: AdminSubscriptionsBrakedInput{
			Site: SampleSite, From: SampleFrom, AdminAddress: sampleAdmin, Hour: "2026-11-05T10:00:00Z",
		}},
	}
}

// AdminSubscriptionsBraked tells the admin that email subscriptions were braked
// in the hour of at.
func AdminSubscriptionsBraked(site links.Site, from, adminAddress string, at time.Time) *Envelope {
	return adminSubscriptionsBrakedMail.MustRender(AdminSubscriptionsBrakedInput{
		Site: site, From: from, AdminAddress: adminAddress, Hour: at.UTC().Truncate(time.Hour).Format(time.RFC3339),
	})
}
