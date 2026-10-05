package accounts_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
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
	require.Equal(t, "curious", lo.FromPtr(row.Reason))
	require.NotNil(t, row.VerificationSentAt, "the confirmation is marked as sent")
	// the column has no time zone: it reads back as the wall clock time it was written at
	now := time.Now()
	wall := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
	require.WithinDuration(t, wall, lo.FromPtr(row.VerificationSentAt), 5*time.Second)
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
		require.NotNil(t, testutil.Must(factory.GetSignupRequest(ctx, db, req.ID))(t).EmailConfirmedAt)
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
	require.Equal(t, inviter.ID, got.User.ID, "the inviter comes with it")

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

	for name, act := range map[string]func(*model.User) error{
		"GenerateAPIKey":      func(u *model.User) error { return svc.GenerateAPIKey(ctx, u) },
		"RegenerateFeedToken": func(u *model.User) error { return svc.RegenerateFeedToken(ctx, u) },
		"SaveUserStyles":      func(u *model.User) error { return svc.SaveUserStyles(ctx, u, "x") },
		"SaveGeneralSettings": func(u *model.User) error {
			return svc.SaveGeneralSettings(ctx, u, "UTC", model.ProfileVisibilityPublic)
		},
		"SendInvite": func(u *model.User) error { return svc.SendInvite(ctx, u, "a@example.test") },
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

	short := accounts.New(repo.New(db), nil, nil, accounts.WithUserStylesMaxLength(5))
	require.Equal(t, 5, short.TextLimits().UserStylesMaxLength)
	require.NoError(t, short.SaveUserStyles(ctx, user, " a{b} "))

	var invalid *service.ValidationError
	require.ErrorAs(t, short.SaveUserStyles(ctx, user, "a{bc}d"), &invalid, "the configured limit applies")
	require.Equal(t, "styles", invalid.Field)

	css, err = svc.UserStyles(ctx, "nobody-has-this-name")
	require.NoError(t, err)
	require.Empty(t, css)
}

func TestSettingsChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)
	user := testutil.Must(factory.User(ctx, db))(t)

	require.NoError(t, svc.SaveGeneralSettings(ctx, user, "Europe/Berlin", model.ProfileVisibilityPublic))
	got := testutil.Must(factory.GetUser(ctx, db, user.ID))(t)
	require.Equal(t, "Europe/Berlin", got.Timezone)
	require.Equal(t, model.ProfileVisibilityPublic, got.ProfileVisibility)
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

func TestProfileAbout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)
	user := newUser(t, ctx, db)

	about := func() string {
		v, err := svc.Settings(ctx, user)
		require.NoError(t, err)
		return v.ProfileAbout
	}
	rows := func() int64 {
		n, err := repo.Query(db).NewSelect().Model((*model.UserProfile)(nil)).Where("user_id = ?", user.ID).Count(ctx)
		require.NoError(t, err)
		return int64(n)
	}

	require.Empty(t, about())

	require.NoError(t, svc.SaveProfile(ctx, user, "first **text**"))
	require.NoError(t, svc.SaveProfile(ctx, user, "  second  "))
	require.Equal(t, "second", about(), "saving replaces the text")
	require.EqualValues(t, 1, rows())

	require.NoError(t, svc.SaveProfile(ctx, user, ""))
	require.Empty(t, about())
	require.Zero(t, rows(), "an empty text deletes the row")

	require.NoError(t, svc.SaveProfile(ctx, user, "again"))
	require.NoError(t, svc.SaveProfile(ctx, user, " \n\t "))
	require.Zero(t, rows(), "whitespace only counts as empty")

	limit := accounts.DefaultProfileAboutMaxLength
	require.NoError(t, svc.SaveProfile(ctx, user, strings.Repeat("я", limit)))
	err := svc.SaveProfile(ctx, user, strings.Repeat("я", limit+1))
	var invalid *service.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, "about", invalid.Field)
	require.Equal(t, strings.Repeat("я", limit), about(), "a rejected text changes nothing")

	short := accounts.New(repo.New(db), nil, nil, accounts.WithProfileAboutMaxLength(5))
	require.NoError(t, short.SaveProfile(ctx, user, "12345"))
	require.ErrorAs(t, short.SaveProfile(ctx, user, "123456"), &invalid, "the configured limit applies")

	require.ErrorIs(t, svc.SaveProfile(ctx, nil, "hi"), service.ErrNeedsLogin)
}
