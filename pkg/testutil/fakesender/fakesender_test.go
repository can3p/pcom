package fakesender_test

import (
	"context"
	"errors"
	"net/mail"
	"testing"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/stretchr/testify/require"
)

func TestSender_RecordsMail(t *testing.T) {
	t.Parallel()

	s := fakesender.New()

	m := &sender.Mail{
		From:    mail.Address{Address: "from@example.com"},
		To:      []mail.Address{{Address: "to@example.com"}},
		Subject: "hi",
	}

	require.NoError(t, s.Send(context.Background(), nil, "unique-1", "welcome", m))

	other := &sender.Mail{Subject: "second"}
	require.NoError(t, s.Send(context.Background(), nil, "unique-2", "confirm_signup", other))

	sent := s.Sent()
	require.Len(t, sent, 2)

	require.Equal(t, "unique-1", sent[0].UniqueID)
	require.Equal(t, "welcome", sent[0].EmailType)
	require.Same(t, m, sent[0].Mail)

	require.Equal(t, "unique-2", sent[1].UniqueID)
	require.Equal(t, "confirm_signup", sent[1].EmailType)
	require.Same(t, other, sent[1].Mail)
}

func TestSender_FailWith(t *testing.T) {
	t.Parallel()

	s := fakesender.New()
	boom := errors.New("boom")
	s.FailWith(boom)

	err := s.Send(context.Background(), nil, "unique-3", "welcome", &sender.Mail{})
	require.ErrorIs(t, err, boom)
	require.Empty(t, s.Sent(), "a failed send should not be recorded")

	s.FailWith(nil)
	require.NoError(t, s.Send(context.Background(), nil, "unique-4", "welcome", &sender.Mail{}))
	require.Len(t, s.Sent(), 1, "recording resumes once FailWith(nil) clears the error")
}
