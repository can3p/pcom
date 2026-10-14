package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/google/uuid"
)

// AdminThrowAwayEmailInput is what the throwaway email alert to the admin
// shows.
type AdminThrowAwayEmailInput struct {
	// Site is the site the mail's links, header and footer point to.
	Site         links.Site
	From         string
	AdminAddress string
	UniqueID     string
	Email        string
}

// Header addresses the mail to the admin; every attempt has its own id.
func (in AdminThrowAwayEmailInput) Header() Header {
	return Header{Site: in.Site, UniqueID: in.UniqueID, From: FromPcom(in.From), To: To(in.AdminAddress)}
}

var adminThrowAwayEmailMail = declare("throw_away_email_signup", "admin_throwaway_email", adminThrowAwayEmailSamples)

func adminThrowAwayEmailSamples() []Sample[AdminThrowAwayEmailInput] {
	return []Sample[AdminThrowAwayEmailInput]{
		{Name: "admin_throwaway_email", Input: AdminThrowAwayEmailInput{
			Site: SampleSite, From: SampleFrom, AdminAddress: sampleAdmin,
			UniqueID: "0190a3b4-0000-7000-8000-000000000003", Email: "test@throwaway.example.com",
		}},
	}
}

// AdminThrowAwayEmailSignupAttempt tells the admin that somebody tried to
// sign up with a throwaway email domain.
func AdminThrowAwayEmailSignupAttempt(site links.Site, from, adminAddress string, email string) *Envelope {
	return adminThrowAwayEmailMail.MustRender(AdminThrowAwayEmailInput{
		Site: site, From: from, AdminAddress: adminAddress, UniqueID: uuid.NewString(), Email: email,
	})
}
