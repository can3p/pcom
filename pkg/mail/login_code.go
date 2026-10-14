package mail

import (
	"time"

	"github.com/can3p/pcom/pkg/links"
)

// LoginCodeInput is what the login code mail shows: the code of one login
// attempt and how long it works.
type LoginCodeInput struct {
	// Site is the site the mail's links, header and footer point to.
	Site      links.Site
	From      string
	AttemptID string
	To        string
	Code      string
	Lifetime  time.Duration
}

// Header addresses the mail to the address the attempt was started with,
// unique per attempt.
func (in LoginCodeInput) Header() Header {
	return Header{Site: in.Site, UniqueID: in.AttemptID, From: FromPcom(in.From), To: To(in.To)}
}

// Minutes is the lifetime of the code in whole minutes.
func (in LoginCodeInput) Minutes() int {
	return int(in.Lifetime.Minutes())
}

var loginCodeMail = declare("login_code", "login_code", loginCodeSamples)

func loginCodeSamples() []Sample[LoginCodeInput] {
	return []Sample[LoginCodeInput]{
		{Name: "login_code", Input: LoginCodeInput{
			Site: SampleSite, From: SampleFrom, AttemptID: "attempt-1", To: "user@example.test", Code: "123456", Lifetime: 15 * time.Minute,
		}},
	}
}

// LoginCode is the mail that carries the code for one login attempt to the
// address it was started with.
func LoginCode(site links.Site, from string, attemptID, to, code string, lifetime time.Duration) *Envelope {
	return loginCodeMail.MustRender(LoginCodeInput{Site: site, From: from, AttemptID: attemptID, To: to, Code: code, Lifetime: lifetime})
}
