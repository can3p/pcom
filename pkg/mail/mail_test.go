package mail_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

const testFrom = "noreply@pcom.test"

// setPostURL sets the URL relation on a post, as if it had been loaded.
func setPostURL(post *model.Post, url *model.NormalizedURL) {
	post.URL = url
}

// deliver queues out on s, the way the posts service does, so the tests read
// what a recipient would get. A nil out sends nothing.
func deliver(ctx context.Context, s *fakesender.Sender) func(*mail.Outgoing) error {
	return func(out *mail.Outgoing) error {
		if out == nil {
			return nil
		}

		return s.Send(ctx, nil, out.UniqueID, out.Type, out.Mail)
	}
}

// deliverE is deliver for the formatters that can fail.
func deliverE(ctx context.Context, s *fakesender.Sender) func(*mail.Outgoing, error) error {
	return func(out *mail.Outgoing, err error) error {
		if err != nil {
			return err
		}

		return deliver(ctx, s)(out)
	}
}

// mailsToGolden serializes fakesender recordings to a human-readable format for golden testing
func mailsToGolden(sent []fakesender.Recorded) []byte {
	var buf bytes.Buffer
	for i, rec := range sent {
		if i > 0 {
			buf.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&buf, "EmailType: %s\n", rec.EmailType)
		fmt.Fprintf(&buf, "UniqueID: %s\n", rec.UniqueID)
		fmt.Fprintf(&buf, "From: %s <%s>\n", rec.Mail.From.Name, rec.Mail.From.Address)
		if len(rec.Mail.To) > 0 {
			fmt.Fprintf(&buf, "To: %s\n", rec.Mail.To[0].Address)
		}
		fmt.Fprintf(&buf, "Subject: %s\n", rec.Mail.Subject)
		fmt.Fprintf(&buf, "\n--- TEXT ---\n%s\n", rec.Mail.Text)
		fmt.Fprintf(&buf, "\n--- HTML ---\n%s\n", rec.Mail.Html)
	}
	return buf.Bytes()
}

// send queues an envelope the way a service does, through the sender with no database.
func send(ctx context.Context, s *fakesender.Sender, e *mail.Envelope) error {
	return s.Send(ctx, nil, e.UniqueID, e.Type, e.Mail)
}

func TestConfirmSignup(t *testing.T) {
	t.Parallel()

	sender := fakesender.New()

	require.NoError(t, send(context.Background(), sender, mail.ConfirmSignup(testFrom, "attempt-1", "user@example.test", "123456", 15*time.Minute)))

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "confirm_signup", mailsToGolden(sent))
}

func TestLoginCode(t *testing.T) {
	t.Parallel()

	sender := fakesender.New()
	require.NoError(t, send(context.Background(), sender, mail.LoginCode(testFrom, "attempt-1", "user@example.test", "123456", 15*time.Minute)))

	golden.Assert(t, "login_code", mailsToGolden(sender.Sent()))
}
