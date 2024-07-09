package admin

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model/core"
)

var NotifyAddress string = os.Getenv("ADMIN_ADDRESS")

func NotifyNewUser(ctx context.Context, s sender.Sender, user *core.User) {
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
	* Email: %s`, user.ID, user.Email),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>New user alert:</p>

	<ul>
		<li>ID: %s</li>
		<li>Email: %s</li>
	</ul>`, user.ID, user.Email),
	}

	err := s.Send(ctx, mail)

	if err != nil {
		log.Fatal(err)
	}
}

func NotifyNewWaitingListMember(ctx context.Context, s sender.Sender, waitingList *core.UserSignupRequest) {
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

	err := s.Send(ctx, mail)

	if err != nil {
		log.Fatal(err)
	}
}

func NotifySignupConfirmed(ctx context.Context, s sender.Sender, user *core.User) {
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

	err := s.Send(ctx, mail)

	if err != nil {
		log.Fatal(err)
	}
}

func NotifyThrowAwayEmailSignupAttempt(ctx context.Context, s sender.Sender, email string) {
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

	err := s.Send(ctx, mail)

	if err != nil {
		log.Fatal(err)
	}
}
