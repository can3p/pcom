package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/samber/lo"
)

const noReasonGiven = "Not specified"

// AdminNewWaitingListMemberInput is what the waiting list alert to the admin
// shows.
type AdminNewWaitingListMemberInput struct {
	// Site is the site the mail's links, header and footer point to.
	Site         links.Site
	From         string
	AdminAddress string
	ID           string
	Email        string
	Reason       string
}

// Header addresses the mail to the admin, unique per waiting list entry.
func (in AdminNewWaitingListMemberInput) Header() Header {
	return Header{Site: in.Site, UniqueID: in.ID, From: FromPcom(in.From), To: To(in.AdminAddress)}
}

var adminNewWaitingListMemberMail = declare("new_waiting_list_member", "admin_new_waiting_list_member", adminNewWaitingListMemberSamples)

func adminNewWaitingListMemberSamples() []Sample[AdminNewWaitingListMemberInput] {
	return []Sample[AdminNewWaitingListMemberInput]{
		{Name: "admin_new_waiting_list_member", Input: AdminNewWaitingListMemberInput{
			Site: SampleSite, From: SampleFrom, AdminAddress: sampleAdmin,
			ID: "0190a3b4-0000-7000-8000-000000000002", Email: "signup@example.test", Reason: noReasonGiven,
		}},
		{Name: "admin_new_waiting_list_member_reason", Input: AdminNewWaitingListMemberInput{
			Site: SampleSite, From: SampleFrom, AdminAddress: sampleAdmin,
			ID: "0190a3b4-0000-7000-8000-000000000002", Email: "signup@example.test", Reason: "I read your blog",
		}},
	}
}

// AdminNewWaitingListMember tells the admin about a new waiting list entry.
func AdminNewWaitingListMember(site links.Site, from, adminAddress string, waitingList *model.UserSignupRequest) *Envelope {
	reason := lo.FromPtr(waitingList.Reason)
	if reason == "" {
		reason = noReasonGiven
	}

	return adminNewWaitingListMemberMail.MustRender(AdminNewWaitingListMemberInput{
		Site: site, From: from, AdminAddress: adminAddress,
		ID: waitingList.ID, Email: waitingList.Email, Reason: reason,
	})
}
