package admin_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// normalizeUUID replaces UUIDs with a placeholder for golden test consistency
func normalizeUUID(content string) string {
	// Match UUID pattern: 8-4-4-4-12 hex digits
	uuidPattern := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	return uuidPattern.ReplaceAllString(content, "[UUID]")
}

func TestNotifyNewUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	sender := fakesender.New()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	admin.NotifyNewUser(ctx, db, sender, user)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	require.Equal(t, "admin_new_user", sent[0].EmailType)
	require.Equal(t, "New User on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_new_user_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_new_user_html", []byte(normalizeUUID(mail.Html)))
}

func TestNotifyNewWaitingListMember(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	sender := fakesender.New()

	signup, err := factory.SignupRequest(ctx, db)
	require.NoError(t, err)

	admin.NotifyNewWaitingListMember(ctx, db, sender, signup)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	require.Equal(t, "new_waiting_list_member", sent[0].EmailType)
	require.Equal(t, "New waiting list member on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_new_waiting_list_member_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_new_waiting_list_member_html", []byte(normalizeUUID(mail.Html)))
}

func TestNotifySignupConfirmed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	sender := fakesender.New()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	admin.NotifySignupConfirmed(ctx, db, sender, user)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	require.Equal(t, "signup_confirmed", sent[0].EmailType)
	require.Equal(t, "New User confirmed email on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_signup_confirmed_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_signup_confirmed_html", []byte(normalizeUUID(mail.Html)))
}

func TestNotifyThrowAwayEmailSignupAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	sender := fakesender.New()

	email := "test@throwaway.example.com"
	admin.NotifyThrowAwayEmailSignupAttempt(ctx, db, sender, email)

	sent := sender.Sent()
	require.Len(t, sent, 1)

	mail := sent[0].Mail
	require.Equal(t, "throw_away_email_signup", sent[0].EmailType)
	require.Equal(t, "An attempt to use a throwaway email domain on pcom", mail.Subject)

	// Golden test for text content
	golden.Assert(t, "notify_throwaway_email_text", []byte(normalizeUUID(mail.Text)))
	golden.Assert(t, "notify_throwaway_email_html", []byte(normalizeUUID(mail.Html)))
}
