package mail

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

func init() {
	if os.Getenv("SENDER_ADDRESS") == "" {
		_ = os.Setenv("SENDER_ADDRESS", "noreply@pcom.test")
	}
}

// mailsToGolden serializes fakesender recordings to a human-readable format for golden testing
func mailsToGoldenInternal(sent []fakesender.Recorded) []byte {
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

func TestSendActualConfirmWaitingList(t *testing.T) {
	t.Parallel()

	waitingList := &core.UserSignupRequest{
		ID:    "request-1",
		Email: "newuser@example.test",
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := sendActualConfirmWaitingList(ctx, nil, sender, waitingList)
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "send_actual_confirm_waiting_list", mailsToGoldenInternal(sent))
}

func TestSendActualInvitation(t *testing.T) {
	t.Parallel()

	user := &core.User{
		ID:       "user-1",
		Email:    "sender@example.test",
		Username: "alice",
	}
	invite := &core.UserInvitation{
		ID:     "invite-1",
		UserID: user.ID,
	}

	sender := fakesender.New()
	ctx := context.Background()

	err := sendActualInvitation(ctx, nil, sender, invite, user, "newuser@example.test")
	require.NoError(t, err)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	golden.Assert(t, "send_actual_invitation", mailsToGoldenInternal(sent))
}

func TestValidate_InvalidFormat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	err := Validate(ctx, nil, "not-a-valid-email")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid email format")
}

func TestValidate_ValidFormat(t *testing.T) {
	t.Parallel()

	// With nil db, this will panic on the ExistsP call, so we test just the format validation
	// by passing an invalid format that gets caught before the DB query
	// The actual db check would be tested in package tests with a real database
	t.Skip("full validation requires database; format check is validated by TestValidate_InvalidFormat")
}
