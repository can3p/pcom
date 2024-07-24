package mail

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"

	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func ConfirmWaitingList(ctx context.Context, db boil.ContextExecutor, s sender.Sender, waitingList *core.UserSignupRequest) error {
	waitingList.VerificationSentAt = null.TimeFrom(time.Now())

	waitingList.UpdateP(ctx, db, boil.Infer())

	return sendActualConfirmWaitingList(ctx, db, s, waitingList)
}

func sendActualConfirmWaitingList(ctx context.Context, db boil.ContextExecutor, s sender.Sender, waitingList *core.UserSignupRequest) error {
	link := links.AbsLink("confirm_waiting_list", waitingList.ID)
	to := waitingList.Email

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: to,
			},
		},
		Subject: "Waiting list on pcom",
		Text: fmt.Sprintf(`
	Hi!

	Thank you for your interest pcom! Please follow the link to confirm your email address

	%s`, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Thank you for your interest pcom! Please follow the link to confirm your email address</p>

	<a href="%s">%s</a>`, link, link),
	}

	err := s.Send(ctx, db, waitingList.ID, "waiting_list_confirm", mail)

	if err != nil {
		log.Fatal(err)
	}

	return nil
}
