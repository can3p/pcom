package mail

import "github.com/can3p/pcom/pkg/model"

// AdminSignupConfirmedInput is what the confirmed account alert to the admin
// shows.
type AdminSignupConfirmedInput struct {
	From         string
	AdminAddress string
	ID           string
	Email        string
}

// Header addresses the mail to the admin, unique per account.
func (in AdminSignupConfirmedInput) Header() Header {
	return Header{UniqueID: in.ID, From: FromPcom(in.From), To: To(in.AdminAddress)}
}

var adminSignupConfirmedMail = declare("signup_confirmed", "admin_signup_confirmed", adminSignupConfirmedSamples)

func adminSignupConfirmedSamples() []Sample[AdminSignupConfirmedInput] {
	return []Sample[AdminSignupConfirmedInput]{
		{Name: "admin_signup_confirmed", Input: AdminSignupConfirmedInput{
			From: SampleFrom, AdminAddress: sampleAdmin,
			ID: "0190a3b4-0000-7000-8000-000000000001", Email: "alice@example.test",
		}},
	}
}

// AdminSignupConfirmed tells the admin that an account confirmed its email.
func AdminSignupConfirmed(from, adminAddress string, user *model.User) *Envelope {
	return adminSignupConfirmedMail.MustRender(AdminSignupConfirmedInput{
		From: from, AdminAddress: adminAddress, ID: user.ID, Email: user.Email,
	})
}
