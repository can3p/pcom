package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// sampleAdmin is the admin address the samples of the admin mails go to.
const sampleAdmin = "admin@pcom.test"

// AdminNewUserInput is what the new account alert to the admin shows.
type AdminNewUserInput struct {
	From         string
	AdminAddress string
	ID           string
	BlogURL      string
	Email        string
}

// Header addresses the mail to the admin, unique per account.
func (in AdminNewUserInput) Header() Header {
	return Header{UniqueID: in.ID, From: FromPcom(in.From), To: To(in.AdminAddress)}
}

var adminNewUserMail = declare("admin_new_user", "admin_new_user", adminNewUserSamples)

func adminNewUserSamples() []Sample[AdminNewUserInput] {
	return []Sample[AdminNewUserInput]{
		{Name: "admin_new_user", Input: AdminNewUserInput{
			From: SampleFrom, AdminAddress: sampleAdmin,
			ID: "0190a3b4-0000-7000-8000-000000000001", BlogURL: SampleSite.Abs("user", "alice"), Email: "alice@example.test",
		}},
	}
}

// AdminNewUser tells the admin about a new account.
func AdminNewUser(site links.Site, from, adminAddress string, user *model.User) *Envelope {
	return adminNewUserMail.MustRender(AdminNewUserInput{
		From: from, AdminAddress: adminAddress,
		ID: user.ID, BlogURL: site.Abs("user", user.Username), Email: user.Email,
	})
}
