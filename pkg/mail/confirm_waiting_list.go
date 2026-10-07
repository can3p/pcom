package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// ConfirmWaitingListInput is what the waiting list confirmation mail shows:
// the link that confirms the entry's email address.
type ConfirmWaitingListInput struct {
	Site      links.Site
	From      string
	RequestID string
	To        string
}

// Header addresses the mail to the entry's address, unique per entry.
func (in ConfirmWaitingListInput) Header() Header {
	return Header{UniqueID: in.RequestID, From: FromPcom(in.From), To: To(in.To)}
}

// Link is the absolute link that confirms the entry.
func (in ConfirmWaitingListInput) Link() string {
	return in.Site.Abs("confirm_waiting_list", in.RequestID)
}

var confirmWaitingListMail = declare("waiting_list_confirm", "confirm_waiting_list", confirmWaitingListSamples)

func confirmWaitingListSamples() []Sample[ConfirmWaitingListInput] {
	return []Sample[ConfirmWaitingListInput]{
		{Name: "send_actual_confirm_waiting_list", Input: ConfirmWaitingListInput{
			Site: SampleSite, From: SampleFrom, RequestID: "request-1", To: "newuser@example.test",
		}},
	}
}

// ConfirmWaitingList is the mail with the link that confirms a waiting list
// entry's email address.
func ConfirmWaitingList(site links.Site, from string, waitingList *model.UserSignupRequest) *Envelope {
	return confirmWaitingListMail.MustRender(ConfirmWaitingListInput{
		Site: site, From: from, RequestID: waitingList.ID, To: waitingList.Email,
	})
}
