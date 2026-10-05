package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsers_Lookups(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	u := insertUser(t, repo.Query(db))

	got, err := store.UserByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.Username, got.Username)

	_, err = store.UserByID(ctx, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	got, err = store.UserByEmail(ctx, u.Email)
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)

	got, err = store.UserByUsername(ctx, u.Username)
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)

	_, err = store.UserByUsername(ctx, "nobody-"+uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	ok, err := store.UserEmailExists(ctx, u.Email)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = store.MailboxHasAccount(ctx, u.EmailCanonical)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = store.UsernameExists(ctx, "nobody-"+uuid.NewString())
	require.NoError(t, err)
	require.False(t, ok)

	// the second user is created later, so it comes first
	u2 := insertUser(t, repo.Query(db))
	users, err := store.UsersByIDs(ctx, []string{u.ID, u2.ID})
	require.NoError(t, err)
	require.Len(t, users, 2)

	users, err = store.UsersByIDsNewestFirst(ctx, []string{u.ID, u2.ID})
	require.NoError(t, err)
	require.Len(t, users, 2)
	require.Equal(t, u2.ID, users[0].ID)

	users, err = store.UsersByIDs(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, users)
}

func TestUsers_SaveUserWritesOnlyTheNamedColumns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	u := insertUser(t, repo.Query(db))

	u.Timezone = "Europe/Berlin"
	u.Username = "changed-" + uuid.NewString()[:6]
	require.NoError(t, store.SaveUser(ctx, u, "timezone"))

	got, err := store.UserByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "Europe/Berlin", got.Timezone)
	require.NotEqual(t, u.Username, got.Username)

	require.NoError(t, store.SaveUser(ctx, u))
	got, err = store.UserByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.Username, got.Username)
}

func TestUsers_UnconfirmedPruning(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	q := repo.Query(db)

	plain := insertUser(t, q)
	confirmed := insertUser(t, q)
	now := time.Now()
	confirmed.EmailConfirmedAt = &now
	require.NoError(t, store.SaveUser(ctx, confirmed, "email_confirmed_at"))
	invited := insertUser(t, q)
	require.NoError(t, store.InsertInvitation(ctx, &model.UserInvitation{ID: uuid.NewString(), UserID: invited.ID}))

	ids, err := store.UnconfirmedUserIDsCreatedBefore(ctx, now.Add(time.Hour))
	require.NoError(t, err)
	require.Contains(t, ids, plain.ID)
	require.NotContains(t, ids, confirmed.ID)
	require.NotContains(t, ids, invited.ID)

	ids, err = store.UnconfirmedUserIDsCreatedBefore(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	require.NotContains(t, ids, plain.ID)

	deleted, err := store.DeleteUnconfirmedUser(ctx, confirmed.ID)
	require.NoError(t, err)
	require.False(t, deleted)

	deleted, err = store.DeleteUnconfirmedUser(ctx, plain.ID)
	require.NoError(t, err)
	require.True(t, deleted)
}

func TestProfiles_AboutUpsert(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	u := insertUser(t, repo.Query(db))

	about, err := store.ProfileAbout(ctx, u.ID)
	require.NoError(t, err)
	require.Empty(t, about)

	require.NoError(t, store.SaveProfileAbout(ctx, u.ID, "one"))
	require.NoError(t, store.SaveProfileAbout(ctx, u.ID, "two"))
	about, err = store.ProfileAbout(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "two", about)

	require.NoError(t, store.DeleteProfileAbout(ctx, u.ID))
	require.NoError(t, store.DeleteProfileAbout(ctx, u.ID))
	about, err = store.ProfileAbout(ctx, u.ID)
	require.NoError(t, err)
	require.Empty(t, about)
}

func TestAPIKeys_OnePerUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	u := insertUser(t, repo.Query(db))

	k, err := store.APIKeyForUser(ctx, u.ID)
	require.NoError(t, err)
	require.Nil(t, k)

	require.NoError(t, store.CreateAPIKey(ctx, u.ID))
	k, err = store.APIKeyForUser(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, k)

	require.NoError(t, store.CreateAPIKey(ctx, u.ID))
	again, err := store.APIKeyForUser(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, k.APIKey, again.APIKey)

	owner, err := store.UserByAPIKey(ctx, k.APIKey)
	require.NoError(t, err)
	require.Equal(t, u.ID, owner.ID)

	_, err = store.UserByAPIKey(ctx, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestSettings_RegistrationAndStyles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	u := insertUser(t, repo.Query(db))

	require.NoError(t, store.SetRegistrationOpen(ctx, true))
	open, err := store.RegistrationOpen(ctx)
	require.NoError(t, err)
	require.True(t, open)
	require.NoError(t, store.SetRegistrationOpen(ctx, false))
	open, err = store.RegistrationOpen(ctx)
	require.NoError(t, err)
	require.False(t, open)

	st, err := store.UserStyleForUser(ctx, u.ID)
	require.NoError(t, err)
	require.Nil(t, st)

	st, err = store.UserStyleByUsername(ctx, "nobody-"+uuid.NewString())
	require.NoError(t, err)
	require.Nil(t, st)

	require.NoError(t, store.SaveUserStyle(ctx, u.ID, "a{}"))
	require.NoError(t, store.SaveUserStyle(ctx, u.ID, "b{}"))

	st, err = store.UserStyleByUsername(ctx, u.Username)
	require.NoError(t, err)
	require.NotNil(t, st)
	require.Equal(t, "b{}", st.Styles)
}

func TestInvites_Filters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	inviter := insertUser(t, repo.Query(db))
	invitee := insertUser(t, repo.Query(db))

	email := "x" + uuid.NewString()[:8] + "@example.com"
	unused := &model.UserInvitation{ID: uuid.NewString(), UserID: inviter.ID}
	require.NoError(t, store.InsertInvitation(ctx, unused))
	sent := &model.UserInvitation{ID: uuid.NewString(), UserID: inviter.ID, InvitationEmail: &email}
	require.NoError(t, store.InsertInvitation(ctx, sent))

	n, err := store.InvitationCount(ctx, inviter.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	list, err := store.SentInvitations(ctx, inviter.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, sent.ID, list[0].ID)

	ok, err := store.PendingInvitationExists(ctx, email)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = store.InvitationSentTo(ctx, email)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := store.OpenInvitationByID(ctx, sent.ID)
	require.NoError(t, err)
	require.NotNil(t, got.User)
	require.Equal(t, inviter.ID, got.User.ID)

	sent.CreatedUserID = &invitee.ID
	require.NoError(t, store.SaveInvitation(ctx, sent))
	_, err = store.OpenInvitationByID(ctx, sent.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	ok, err = store.PendingInvitationExists(ctx, email)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = store.InvitationSentTo(ctx, email)
	require.NoError(t, err)
	require.True(t, ok)

	// a row locked by one transaction is skipped by another
	err = store.Tx(ctx, func(tx *repo.Store) error {
		locked, err := tx.LockUnusedInvitation(ctx, inviter.ID)
		require.NoError(t, err)
		require.Equal(t, unused.ID, locked.ID)

		return repo.New(db).Tx(ctx, func(other *repo.Store) error {
			_, err := other.LockUnusedInvitation(ctx, inviter.ID)
			require.ErrorIs(t, err, repo.ErrNotFound)

			return nil
		})
	})
	require.NoError(t, err)
}

func TestInvites_SignupRequests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)
	email := "w" + uuid.NewString()[:8] + "@example.com"

	r := &model.UserSignupRequest{ID: uuid.NewString(), Email: email}
	require.NoError(t, store.InsertSignupRequest(ctx, r))
	require.NoError(t, store.InsertSignupRequest(ctx, &model.UserSignupRequest{ID: uuid.NewString(), Email: email}))

	ok, err := store.SignupRequestEmailExists(ctx, email)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := store.SignupRequestByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, email, got.Email)

	now := time.Now()
	got.EmailConfirmedAt = &now
	require.NoError(t, store.SaveSignupRequest(ctx, got))
	got, err = store.SignupRequestByID(ctx, r.ID)
	require.NoError(t, err)
	require.NotNil(t, got.EmailConfirmedAt)

	_, err = store.SignupRequestByID(ctx, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestLoginAttempts_Queries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	u := insertUser(t, repo.Query(db))
	other := insertUser(t, repo.Query(db))

	code := "hash"
	now := time.Now()
	mk := func(userID string, hash *string, tries int, expires time.Time) *model.LoginAttempt {
		a := &model.LoginAttempt{
			ID: uuid.NewString(), UserID: &userID, CodeHash: hash, WrongTries: tries, ExpiresAt: expires,
		}
		require.NoError(t, store.InsertLoginAttempt(ctx, a))

		return a
	}

	first := mk(u.ID, &code, 2, now.Add(time.Hour))
	second := mk(u.ID, nil, 3, now.Add(time.Hour))
	expired := mk(u.ID, &code, 4, now.Add(-time.Hour))
	mk(other.ID, &code, 7, now.Add(time.Hour))

	since := now.Add(-time.Minute)
	n, err := store.LoginCodesSince(ctx, u.ID, since)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	n, err = store.WrongLoginTriesSince(ctx, u.ID, since)
	require.NoError(t, err)
	require.EqualValues(t, 9, n)

	n, err = store.WrongLoginTriesSince(ctx, uuid.NewString(), since)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)

	list, err := store.UnexpiredLoginAttempts(ctx, u.ID, now)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, second.ID, list[0].ID)
	require.Equal(t, first.ID, list[1].ID)

	// only the named columns (and updated_at) are written
	first.WrongTries = 5
	first.ReturnURL = "/elsewhere"
	require.NoError(t, store.SaveLoginAttempt(ctx, first, "wrong_tries"))
	got, err := store.LockLoginAttempt(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, 5, got.WrongTries)
	require.Empty(t, got.ReturnURL)

	require.NoError(t, store.SaveLoginAttempt(ctx, first))
	got, err = store.LockLoginAttempt(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "/elsewhere", got.ReturnURL)

	_, err = store.LockLoginAttempt(ctx, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	deleted, err := store.DeleteLoginAttemptsExpiredBefore(ctx, now)
	require.NoError(t, err)
	require.GreaterOrEqual(t, deleted, int64(1))
	_, err = store.LockLoginAttempt(ctx, expired.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestLocks_OnlyInsideATransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)

	require.Error(t, store.LockUser(ctx, repo.LockLoginCodes, "u"))
	require.NoError(t, store.Tx(ctx, func(tx *repo.Store) error {
		require.NoError(t, tx.LockUser(ctx, repo.LockLoginCodes, "u"))

		return tx.LockMailbox(ctx, repo.LockSignupMailbox, "m@example.com")
	}))
}
