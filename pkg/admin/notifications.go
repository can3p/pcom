package admin

import (
	"fmt"
	"html"
	"net/mail"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	pcommail "github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// NewUser tells the admin about a new account.
func NewUser(site links.Site, from, adminAddress string, user *model.User) *pcommail.Envelope {
	blogURL := site.Abs("user", user.Username)

	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: adminAddress,
			},
		},
		Subject: "New User on pcom",
		Text: fmt.Sprintf(`
	Hi!

	New user alert:

	* ID: %s
	* Blog: %s
	* Email: %s`, user.ID, blogURL, user.Email),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>New user alert:</p>

	<ul>
		<li>ID: %s</li>
		<li>Blog: <a href="%s">%s</a></li>
		<li>Email: %s</li>
	</ul>`, html.EscapeString(user.ID), html.EscapeString(blogURL), html.EscapeString(blogURL), html.EscapeString(user.Email)),
	}

	return &pcommail.Envelope{UniqueID: user.ID, Type: "admin_new_user", Mail: mail}
}

// NewWaitingListMember tells the admin about a new waiting list entry.
func NewWaitingListMember(site links.Site, from, adminAddress string, waitingList *model.UserSignupRequest) *pcommail.Envelope {
	r := lo.FromPtr(waitingList.Reason)

	if r == "" {
		r = "Not specified"
	}

	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: adminAddress,
			},
		},
		Subject: "New waiting list member on pcom",
		Text: fmt.Sprintf(`
			Hi!

			New waiting list member alert:

			* Email address: %s
			* Reason: %s
			`, waitingList.Email, r),
		Html: fmt.Sprintf(`
			<p>Hi!</p>

			<p>New waiting list member alert:</p>

			<ul>
			<li>Email address: %s</li>
			<li>Reason: %s</li>
			</ul>`,
			html.EscapeString(waitingList.Email), html.EscapeString(r)),
	}

	return &pcommail.Envelope{UniqueID: waitingList.ID, Type: "new_waiting_list_member", Mail: mail}
}

// SignupConfirmed tells the admin that an account confirmed its email.
func SignupConfirmed(site links.Site, from, adminAddress string, user *model.User) *pcommail.Envelope {
	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: adminAddress,
			},
		},
		Subject: "New User confirmed email on pcom",
		Text: fmt.Sprintf(`
	Hi!

	New conrirmed user alert:

	* ID: %s
	* Email: %s`, user.ID, user.Email),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>New conrirmed user alert:</p>

	<ul>
		<li>ID: %s</li>
		<li>Email: %s</li>
	</ul>`, html.EscapeString(user.ID), html.EscapeString(user.Email)),
	}

	return &pcommail.Envelope{UniqueID: user.ID, Type: "signup_confirmed", Mail: mail}
}

// ThrowAwayEmailSignupAttempt tells the admin that somebody tried to sign up
// with a throwaway email domain.
func ThrowAwayEmailSignupAttempt(from, adminAddress string, email string) *pcommail.Envelope {
	mail := &sender.Mail{
		From: mail.Address{
			Address: from,
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: adminAddress,
			},
		},
		Subject: "An attempt to use a throwaway email domain on pcom",
		Text: fmt.Sprintf(`
	Hi!

	A user has just tried to use a throwaway email: %s`, email),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>A user has just tried to use a throwaway email: %s</p>
	`, html.EscapeString(email)),
	}

	return &pcommail.Envelope{UniqueID: uuid.NewString(), Type: "throw_away_email_signup", Mail: mail}
}
