package mail

import "time"

// ConfirmSignupInput is what the signup confirmation mail shows: the code
// that confirms a new account's address and how long it works.
type ConfirmSignupInput struct {
	From      string
	AttemptID string
	To        string
	Code      string
	Lifetime  time.Duration
}

// Header addresses the mail to the new account's address, unique per login
// attempt.
func (in ConfirmSignupInput) Header() Header {
	return Header{UniqueID: in.AttemptID, From: FromPcom(in.From), To: To(in.To)}
}

// Minutes is the lifetime of the code in whole minutes.
func (in ConfirmSignupInput) Minutes() int {
	return int(in.Lifetime.Minutes())
}

var confirmSignupMail = declare("confirm_signup", "confirm_signup", confirmSignupSamples)

func confirmSignupSamples() []Sample[ConfirmSignupInput] {
	return []Sample[ConfirmSignupInput]{
		{Name: "confirm_signup", Input: ConfirmSignupInput{
			From: SampleFrom, AttemptID: "attempt-1", To: "user@example.test", Code: "123456", Lifetime: 15 * time.Minute,
		}},
	}
}

// ConfirmSignup is the mail that carries the code which confirms a new
// account's email address and logs it in, for the login attempt attemptID.
func ConfirmSignup(from string, attemptID, to, code string, lifetime time.Duration) *Envelope {
	return confirmSignupMail.MustRender(ConfirmSignupInput{From: from, AttemptID: attemptID, To: to, Code: code, Lifetime: lifetime})
}
