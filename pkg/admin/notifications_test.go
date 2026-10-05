package admin_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/links"
	pcommail "github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

const (
	testFrom  = "noreply@pcom.test"
	testAdmin = "admin@pcom.test"
)

// send queues the way a service would: through the sender, with no database.
func send(ctx context.Context, s *fakesender.Sender, m *pcommail.Envelope) error {
	return s.Send(ctx, nil, m.UniqueID, m.Type, m.Mail)
}

// normalizeUUID replaces UUIDs with a placeholder for golden test consistency
func normalizeUUID(content string) string {
	// Match UUID pattern: 8-4-4-4-12 hex digits
	uuidPattern := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	return uuidPattern.ReplaceAllString(content, "[UUID]")
}

func TestNotifyNewUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sender := fakesender.New()

	user := &model.User{
		ID:       "0190a3b4-0000-7000-8000-000000000001",
		Email:    "alice@example.test",
		Username: "alice",
	}

	require.NoError(t, send(ctx, sender, admin.NewUser(links.Site{}, testFrom, testAdmin, user)))

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	requireAddresses(t, mail)
	require.Equal(t, "admin_new_user", sent[0].EmailType)
	require.Equal(t, "New User on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_new_user_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_new_user_html", []byte(normalizeUUID(mail.Html)))
}

func TestNotifyNewWaitingListMember(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sender := fakesender.New()

	signup := &model.UserSignupRequest{
		ID:    "0190a3b4-0000-7000-8000-000000000002",
		Email: "signup@example.test",
	}

	require.NoError(t, send(ctx, sender, admin.NewWaitingListMember(links.Site{}, testFrom, testAdmin, signup)))

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	requireAddresses(t, mail)
	require.Equal(t, "new_waiting_list_member", sent[0].EmailType)
	require.Equal(t, "New waiting list member on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_new_waiting_list_member_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_new_waiting_list_member_html", []byte(normalizeUUID(mail.Html)))
}

func TestNotifySignupConfirmed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sender := fakesender.New()

	user := &model.User{
		ID:       "0190a3b4-0000-7000-8000-000000000001",
		Email:    "alice@example.test",
		Username: "alice",
	}

	require.NoError(t, send(ctx, sender, admin.SignupConfirmed(links.Site{}, testFrom, testAdmin, user)))

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	requireAddresses(t, mail)
	require.Equal(t, "signup_confirmed", sent[0].EmailType)
	require.Equal(t, "New User confirmed email on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_signup_confirmed_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_signup_confirmed_html", []byte(normalizeUUID(mail.Html)))
}

func TestNotifyThrowAwayEmailSignupAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sender := fakesender.New()

	email := "test@throwaway.example.com"
	require.NoError(t, send(ctx, sender, admin.ThrowAwayEmailSignupAttempt(testFrom, testAdmin, email)))

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	requireAddresses(t, mail)
	require.Equal(t, "throw_away_email_signup", sent[0].EmailType)
	require.Equal(t, "An attempt to use a throwaway email domain on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_throwaway_email_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_throwaway_email_html", []byte(normalizeUUID(mail.Html)))
}

// notifiers lists every admin notification with all user-controlled fields
// set to hostile markup (a tag plus quotes).
func notifiers() map[string]func(s *fakesender.Sender) error {
	const hostile = `"><b>x</b>`

	ctx := context.Background()
	user := &model.User{ID: "user-1", Email: hostile, Username: hostile}
	signup := &model.UserSignupRequest{ID: "signup-1", Email: hostile}
	signup.Reason = new(hostile)

	return map[string]func(s *fakesender.Sender) error{
		"NewUser": func(s *fakesender.Sender) error {
			return send(ctx, s, admin.NewUser(links.Site{}, testFrom, testAdmin, user))
		},
		"NewWaitingListMember": func(s *fakesender.Sender) error {
			return send(ctx, s, admin.NewWaitingListMember(links.Site{}, testFrom, testAdmin, signup))
		},
		"SignupConfirmed": func(s *fakesender.Sender) error {
			return send(ctx, s, admin.SignupConfirmed(links.Site{}, testFrom, testAdmin, user))
		},
		"ThrowAwayEmail": func(s *fakesender.Sender) error {
			return send(ctx, s, admin.ThrowAwayEmailSignupAttempt(testFrom, testAdmin, hostile))
		},
	}
}

func TestNotifications_EscapeUserContentInHTML(t *testing.T) {
	t.Parallel()

	for name, notify := range notifiers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			require.NoError(t, notify(s))

			sent := s.Sent()
			require.Len(t, sent, 1)

			requireAddresses(t, sent[0].Mail)
			html := sent[0].Mail.Html
			require.NotContains(t, html, "<b>x</b>")
			require.NotContains(t, html, `"><`)
			require.Contains(t, html, "&lt;b&gt;x&lt;/b&gt;")
			require.Contains(t, html, "&#34;")
		})
	}
}

// Every notification goes from the sender address to the admin address.
func requireAddresses(t *testing.T, m *sender.Mail) {
	t.Helper()

	require.Equal(t, testFrom, m.From.Address)
	require.Len(t, m.To, 1)
	require.Equal(t, testAdmin, m.To[0].Address)
}
