package admin

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var NotifyAddress string = os.Getenv("ADMIN_ADDRESS")

func NotifyNewUser(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, user *core.User) {
	blogURL := links.AbsLink("user", user.Username)

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: NotifyAddress,
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
	</ul>`, user.ID, blogURL, blogURL, user.Email),
	}

	err := s.Send(ctx, exec, user.ID, "admin_new_user", mail)

	if err != nil {
		log.Fatal(err)
	}
}

func NotifyNewWaitingListMember(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, waitingList *core.UserSignupRequest) {
	r := waitingList.Reason.String

	if r == "" {
		r = "Not specified"
	}

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: NotifyAddress,
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
			waitingList.Email, r),
	}

	err := s.Send(ctx, exec, waitingList.ID, "new_waiting_list_member", mail)

	if err != nil {
		log.Fatal(err)
	}
}

func NotifySignupConfirmed(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, user *core.User) {
	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: NotifyAddress,
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
	</ul>`, user.ID, user.Email),
	}

	err := s.Send(ctx, exec, user.ID, "signup_confirmed", mail)

	if err != nil {
		log.Fatal(err)
	}
}

func NotifyThrowAwayEmailSignupAttempt(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, email string) {
	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: NotifyAddress,
			},
		},
		Subject: "An attempt to use a throwaway email domain on pcom",
		Text: fmt.Sprintf(`
	Hi!

	A user has just tried to use a throwaway email: %s`, email),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>A user has just tried to use a throwaway email: %s</p>
	`, email),
	}

	err := s.Send(ctx, exec, uuid.NewString(), "throw_away_email_signup", mail)

	if err != nil {
		log.Fatal(err)
	}
}
