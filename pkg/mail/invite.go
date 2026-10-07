package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// InviteInput is what the invitation mail shows: the link that opens the
// invitation, sent to the address it was sent to.
type InviteInput struct {
	Site     links.Site
	From     string
	InviteID string
	To       string
}

// Header addresses the mail to the invited address, unique per invitation.
func (in InviteInput) Header() Header {
	return Header{UniqueID: in.InviteID, From: FromPcom(in.From), To: To(in.To)}
}

// Link is the absolute link that opens the invitation.
func (in InviteInput) Link() string {
	return in.Site.Abs("invite", in.InviteID)
}

var inviteMail = declare("user_invitation", "invite", inviteSamples)

func inviteSamples() []Sample[InviteInput] {
	return []Sample[InviteInput]{
		{Name: "send_actual_invitation", Input: InviteInput{
			Site: SampleSite, From: SampleFrom, InviteID: "invite-1", To: "newuser@example.test",
		}},
	}
}

// Invitation is the mail that carries an invitation link to the address it
// was sent to.
func Invitation(site links.Site, from string, invite *model.UserInvitation, to string) *Envelope {
	return inviteMail.MustRender(InviteInput{Site: site, From: from, InviteID: invite.ID, To: to})
}
