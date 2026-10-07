package mail_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/stretchr/testify/require"
)

const testAdmin = "admin@pcom.test"

// requireSample checks that got is the rendering of the sample called name.
// Mails whose id is generated per send take the sample's id.
func requireSample(t *testing.T, name string, got *mail.Envelope, generatedID bool) {
	t.Helper()

	want := sampleEnvelope(t, name)
	if generatedID {
		require.NotEmpty(t, got.UniqueID)
		got.UniqueID = want.UniqueID
	}

	require.Equal(t, want, got)
}

func TestAdminNewUser(t *testing.T) {
	t.Parallel()

	user := &model.User{ID: "0190a3b4-0000-7000-8000-000000000001", Email: "alice@example.test", Username: "alice"}

	got := mail.AdminNewUser(mail.SampleSite, testFrom, testAdmin, user)
	requireSample(t, "admin_new_user", got, false)
}

func TestAdminNewWaitingListMember(t *testing.T) {
	t.Parallel()

	signup := &model.UserSignupRequest{ID: "0190a3b4-0000-7000-8000-000000000002", Email: "signup@example.test"}

	got := mail.AdminNewWaitingListMember(testFrom, testAdmin, signup)
	requireSample(t, "admin_new_waiting_list_member", got, false)

	signup.Reason = new("I read your blog")
	requireSample(t, "admin_new_waiting_list_member_reason", mail.AdminNewWaitingListMember(testFrom, testAdmin, signup), false)
}

func TestAdminSignupConfirmed(t *testing.T) {
	t.Parallel()

	user := &model.User{ID: "0190a3b4-0000-7000-8000-000000000001", Email: "alice@example.test", Username: "alice"}

	got := mail.AdminSignupConfirmed(testFrom, testAdmin, user)
	requireSample(t, "admin_signup_confirmed", got, false)
}

func TestAdminThrowAwayEmailSignupAttempt(t *testing.T) {
	t.Parallel()

	got := mail.AdminThrowAwayEmailSignupAttempt(testFrom, testAdmin, "test@throwaway.example.com")
	requireSample(t, "admin_throwaway_email", got, true)

	other := mail.AdminThrowAwayEmailSignupAttempt(testFrom, testAdmin, "test@throwaway.example.com")
	require.NotEqual(t, got.UniqueID, other.UniqueID)
}

func TestAdminPageFailure(t *testing.T) {
	t.Parallel()

	user := &model.User{ID: "0190a3b4-0000-7000-8000-000000000001", Email: "alice@example.test"}

	// the sample report is what the router would pass in
	in := mail.SampleAdminPageFailureReport

	got := mail.AdminPageFailure(testFrom, testAdmin, in, user)
	requireSample(t, "admin_page_failure", got, true)

	requireSample(t, "admin_page_failure_anonymous", mail.AdminPageFailure(testFrom, testAdmin, in, nil), true)
}

// adminMailers lists every admin notification with all user-controlled
// fields set to hostile markup.
func adminMailers() map[string]func() *mail.Envelope {
	user := &model.User{ID: "user-1", Email: hostile, Username: hostile}
	signup := &model.UserSignupRequest{ID: "signup-1", Email: hostile, Reason: new(hostile)}

	return map[string]func() *mail.Envelope{
		"AdminNewUser": func() *mail.Envelope {
			return mail.AdminNewUser(links.Site{}, testFrom, testAdmin, user)
		},
		"AdminNewWaitingListMember": func() *mail.Envelope {
			return mail.AdminNewWaitingListMember(testFrom, testAdmin, signup)
		},
		"AdminSignupConfirmed": func() *mail.Envelope {
			return mail.AdminSignupConfirmed(testFrom, testAdmin, user)
		},
		"AdminThrowAwayEmail": func() *mail.Envelope {
			return mail.AdminThrowAwayEmailSignupAttempt(testFrom, testAdmin, hostile)
		},
		"AdminPageFailure": func() *mail.Envelope {
			return mail.AdminPageFailure(testFrom, testAdmin, hostile, user)
		},
	}
}

func TestAdminMails_EscapeUserContentInHTML(t *testing.T) {
	t.Parallel()

	for name, build := range adminMailers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			require.NoError(t, send(context.Background(), s, build()))

			sent := s.Sent()
			require.Len(t, sent, 1)
			require.Equal(t, testFrom, sent[0].Mail.From.Address)
			require.Len(t, sent[0].Mail.To, 1)
			require.Equal(t, testAdmin, sent[0].Mail.To[0].Address)

			html := sent[0].Mail.Html
			require.NotContains(t, html, rawTag)
			require.NotContains(t, html, `"><`)
			require.Contains(t, html, escapedTag)
			require.Contains(t, html, escapedQuo)
		})
	}
}
