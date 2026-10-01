package accounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestJoinWaitingList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	s := fakesender.New()

	require.NoError(t, svcWith(db, s).JoinWaitingList(ctx, " Waiter@Example.test ", "curious", "index_page"))

	sent := s.Sent()
	require.Len(t, sent, 2)
	confirms := 0
	for _, m := range sent {
		if m.EmailType == "waiting_list_confirm" {
			confirms++
		}
	}
	require.Equal(t, 1, confirms)
	require.Equal(t, "waiting_list_confirm", sent[0].EmailType)
	require.Equal(t, "waiter@example.test", sent[0].Mail.To[0].Address)
	require.Equal(t, "new_waiting_list_member", sent[1].EmailType)

	row := testutil.Must(factory.GetSignupRequest(ctx, db, sent[0].UniqueID))(t)
	require.Equal(t, "waiter@example.test", row.Email)
	require.Equal(t, "curious", row.Reason.String)
	require.True(t, row.VerificationSentAt.Valid, "the confirmation is marked as sent")
	// the column has no time zone: it reads back as the wall clock time it was written at
	now := time.Now()
	wall := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
	require.WithinDuration(t, wall, row.VerificationSentAt.Time, 5*time.Second)
}

func TestConfirmations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	s := fakesender.New()
	svc := svcWith(db, s)

	t.Run("a waiting list link confirms the entry", func(t *testing.T) {
		t.Parallel()

		req := testutil.Must(factory.SignupRequest(ctx, db))(t)
		require.NoError(t, svc.ConfirmWaitingList(ctx, req.ID))
		require.NoError(t, svc.ConfirmWaitingList(ctx, req.ID))
		require.True(t, testutil.Must(factory.GetSignupRequest(ctx, db, req.ID))(t).EmailConfirmedAt.Valid)
	})

	t.Run("unknown links are not found", func(t *testing.T) {
		t.Parallel()

		require.ErrorIs(t, svc.ConfirmWaitingList(ctx, "00000000-0000-0000-0000-000000000000"), service.ErrNotFound)
	})
}

func TestInvitation_OnlyOpenOnesResolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)

	inviter := newUser(t, ctx, db)
	open := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("open@example.test")))(t)
	used := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("used@example.test"), factory.UsedBy(newUser(t, ctx, db).ID)))(t)

	got, err := svc.Invitation(ctx, open.ID)
	require.NoError(t, err)
	require.Equal(t, inviter.ID, got.R.User.ID, "the inviter comes with it")

	_, err = svc.Invitation(ctx, used.ID)
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestAPIKeyAndFeedToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)
	user := newUser(t, ctx, db)

	other := newUser(t, ctx, db)
	require.NoError(t, svc.GenerateAPIKey(ctx, other))
	otherKey := testutil.Must(repo.New(db).APIKeyForUser(ctx, other.ID))(t)

	require.NoError(t, svc.GenerateAPIKey(ctx, user))
	require.Equal(t, otherKey.APIKey, testutil.Must(repo.New(db).APIKeyForUser(ctx, other.ID))(t).APIKey, "another user's key is untouched")
	key := testutil.Must(repo.New(db).APIKeyForUser(ctx, user.ID))(t)
	require.NotNil(t, key)

	got, err := svc.UserByAPIKey(ctx, key.APIKey)
	require.NoError(t, err)
	require.Equal(t, user.ID, got.ID)

	require.NoError(t, svc.GenerateAPIKey(ctx, user))
	again := testutil.Must(repo.New(db).APIKeyForUser(ctx, user.ID))(t)
	require.Equal(t, key.APIKey, again.APIKey, "there is no key rotation")

	_, err = svc.UserByAPIKey(ctx, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, service.ErrNotFound)

	require.NoError(t, svc.RegenerateFeedToken(ctx, user))
	first := testutil.Must(repo.FeedTokenForUser(ctx, db, user.ID))(t)
	require.NoError(t, svc.RegenerateFeedToken(ctx, user))
	second := testutil.Must(repo.FeedTokenForUser(ctx, db, user.ID))(t)
	require.NotEqual(t, first.Token, second.Token, "the old feed URL stops working")

	for name, act := range map[string]func(*core.User) error{
		"GenerateAPIKey":      func(u *core.User) error { return svc.GenerateAPIKey(ctx, u) },
		"RegenerateFeedToken": func(u *core.User) error { return svc.RegenerateFeedToken(ctx, u) },
		"SaveUserStyles":      func(u *core.User) error { return svc.SaveUserStyles(ctx, u, "x") },
		"SaveGeneralSettings": func(u *core.User) error { return svc.SaveGeneralSettings(ctx, u, "UTC", core.ProfileVisibilityPublic) },
		"ChangePassword":      func(u *core.User) error { return svc.ChangePassword(ctx, u, "a", "b") },
		"SendInvite":          func(u *core.User) error { return svc.SendInvite(ctx, u, "a@example.test") },
	} {
		require.ErrorIs(t, act(nil), service.ErrNeedsLogin, name)
	}
}

func TestUserStyles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)
	user := newUser(t, ctx, db)

	css, err := svc.UserStyles(ctx, user.Username)
	require.NoError(t, err)
	require.Empty(t, css)

	require.NoError(t, svc.SaveUserStyles(ctx, user, "  a { b: c }  "))
	require.NoError(t, svc.SaveUserStyles(ctx, user, "  d { e: f }  "))

	css, err = svc.UserStyles(ctx, user.Username)
	require.NoError(t, err)
	require.Equal(t, "d { e: f }", css, "saving replaces the styles and the css comes back trimmed")

	css, err = svc.UserStyles(ctx, "nobody-has-this-name")
	require.NoError(t, err)
	require.Empty(t, css)
}

func TestSettingsAndPasswordChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)
	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("old-password")))(t)

	require.NoError(t, svc.SaveGeneralSettings(ctx, user, "Europe/Berlin", core.ProfileVisibilityPublic))
	got := testutil.Must(factory.GetUser(ctx, db, user.ID))(t)
	require.Equal(t, "Europe/Berlin", got.Timezone)
	require.Equal(t, core.ProfileVisibilityPublic, got.ProfileVisibility)

	_, isOK := problem(svc.ChangePassword(ctx, user, "wrong", "new-password-1!"))
	require.False(t, isOK, "the old password has to match")
	require.NoError(t, svc.ChangePassword(ctx, user, "old-password", "new-password-1!"))
	require.NoError(t, svc.CheckCredentials(ctx, user.Email, "new-password-1!"))
	_, isOK = problem(svc.CheckCredentials(ctx, user.Email, "old-password"))
	require.False(t, isOK, "the old password stops working")
}

func TestRegistrationOpenAndInvites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)

	require.NoError(t, svc.SetRegistrationOpen(ctx, true))
	open, err := svc.RegistrationOpen(ctx)
	require.NoError(t, err)
	require.True(t, open)

	require.NoError(t, svc.SetRegistrationOpen(ctx, false))
	open, err = svc.RegistrationOpen(ctx)
	require.NoError(t, err)
	require.False(t, open)

	user := newUser(t, ctx, db)
	require.NoError(t, svc.AddInvites(ctx, " "+user.Email+" ", 3))
	require.Equal(t, int64(3), testutil.Must(repo.New(db).InvitationCount(ctx, user.ID))(t))
	require.ErrorIs(t, svc.AddInvites(ctx, "nobody@example.test", 1), service.ErrNotFound)
}
