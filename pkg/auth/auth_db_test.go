package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

func TestSignup_InsertsUnconfirmedUserAndNotifiesAdmin(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	u, err := auth.Signup(ctx, db, sender, "new-signup@example.test", "newsignup", "s3cr3t-pw", "some-campaign")
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)

	got, err := factory.GetUser(ctx, db, u.ID)
	require.NoError(t, err)
	require.Equal(t, "new-signup@example.test", got.Email)
	require.Equal(t, "newsignup", got.Username)
	require.False(t, got.EmailConfirmedAt.Valid, "signup leaves the email unconfirmed until it's verified")
	require.True(t, got.EmailConfirmSeed.Valid, "a confirmation seed is generated so the user can confirm later")
	require.Equal(t, "some-campaign", got.SignupAttribution.String)

	require.Len(t, sender.Sent(), 1, "signup notifies the admin of the new user")
}

func TestSignup_MissingFieldsReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	_, err := auth.Signup(ctx, db, sender, "", "user", "pw", "")
	require.Error(t, err)

	_, err = auth.Signup(ctx, db, sender, "a@b.example.test", "", "pw", "")
	require.Error(t, err)

	_, err = auth.Signup(ctx, db, sender, "a@b.example.test", "user", "", "")
	require.Error(t, err)

	require.Empty(t, sender.Sent(), "a rejected signup must not notify anyone")
}

func TestSignup_DuplicateEmailReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	existing, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = auth.Signup(ctx, db, sender, existing.Email, "someoneelse", "s3cr3t-pw", "")
	require.Error(t, err, "the email column is unique, so a second signup for it must fail")
	require.Empty(t, sender.Sent(), "a failed signup must not notify anyone")
}

func TestAcceptInvite_CreatesUserAndConnectsToInviter(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee@example.test"))
	require.NoError(t, err)

	require.NoError(t, auth.AcceptInvite(ctx, db, sender, invite, "invitee", "s3cr3t-pw"))

	// AcceptInvite mutates the invite it was given: CreatedUserID is set to
	// the freshly inserted user's ID.
	require.True(t, invite.CreatedUserID.Valid)
	newUserID := invite.CreatedUserID.String

	gotUser, err := factory.GetUser(ctx, db, newUserID)
	require.NoError(t, err)
	require.Equal(t, "invitee", gotUser.Username)
	require.Equal(t, "invitee@example.test", gotUser.Email)
	require.True(t, gotUser.EmailConfirmedAt.Valid, "accepting an invite confirms the email right away")
	require.Equal(t, "accepted_invite", gotUser.SignupAttribution.String)

	connected, err := factory.ConnectionExists(ctx, db, inviter.ID, newUserID)
	require.NoError(t, err)
	require.True(t, connected, "accepting an invite connects the new user to the inviter")

	require.Len(t, sender.Sent(), 1, "accepting an invite notifies the admin of the new user")
}

func TestAcceptInvite_MissingFieldsReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee2@example.test"))
	require.NoError(t, err)

	require.Error(t, auth.AcceptInvite(ctx, db, sender, invite, "", "s3cr3t-pw"))
	require.Error(t, auth.AcceptInvite(ctx, db, sender, invite, "invitee2", ""))

	require.False(t, invite.CreatedUserID.Valid, "a rejected accept must not consume the invite")
	require.Empty(t, sender.Sent())
}

func TestAcceptInvite_DuplicateEmailReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	existing, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent(existing.Email))
	require.NoError(t, err)

	err = auth.AcceptInvite(ctx, db, sender, invite, "someoneelse", "s3cr3t-pw")
	require.Error(t, err, "the email column is unique, so accepting into an already-used email must fail")
	require.False(t, invite.CreatedUserID.Valid)
	require.Empty(t, sender.Sent())
}

func TestCheckCredentials(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db, factory.WithPassword("correct-horse"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	require.NoError(t, auth.CheckCredentials(c, db, user.Email, "correct-horse"))
	require.Error(t, auth.CheckCredentials(c, db, user.Email, "wrong-password"))
	require.Error(t, auth.CheckCredentials(c, db, "unknown@example.test", "correct-horse"))
}

func TestCheckCredentials_DBErrorIsReturnedAsIs(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	db := testDB.DB
	require.NoError(t, db.Close())

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	err := auth.CheckCredentials(c, db, "someone@example.test", "pw")
	require.Error(t, err)
	require.NotEqual(t, "Bad credentials", err.Error(), "a real DB error must not be mistaken for wrong credentials")
}

func TestLogin_SetsSessionOnSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db, factory.WithPassword("s3cr3t-pw"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	require.NoError(t, auth.Login(c, db, user.Email, "s3cr3t-pw"))

	require.Equal(t, user.ID, sessions.Default(c).Get("user"))
}

func TestLogin_BadCredentialsReturnsErrorWithoutSettingSession(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db, factory.WithPassword("s3cr3t-pw"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	err = auth.Login(c, db, user.Email, "wrong-pw")
	require.Error(t, err)
	require.Nil(t, sessions.Default(c).Get("user"))
}

func TestLogin_DBErrorIsReturnedAsIs(t *testing.T) {
	t.Parallel()
	t.Skip("known bug #160: auth.Login panics on a database error instead of returning it")

	testDB := testdb.New(t)
	db := testDB.DB
	require.NoError(t, db.Close())

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	var err error
	require.NotPanics(t, func() {
		err = auth.Login(c, db, "someone@example.test", "pw")
	})
	require.Error(t, err, "a DB error should be returned to the caller, not mistaken for bad credentials")
}

func TestAuth_StaleSessionUserLogsAndContinues(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)
	sess := sessions.Default(c)
	sess.Set("user", "00000000-0000-0000-0000-000000000000")
	require.NoError(t, sess.Save())

	// A session pointing at a user that no longer exists must not abort the
	// request: Auth only logs the pgsession.SetUser failure and lets the
	// request continue as if anonymous.
	auth.Auth(c, db)

	require.False(t, c.IsAborted())
	require.Nil(t, pgsession.GetUser(c))
}

func TestEnforceAuth_LoggedInUserPassesThrough(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodGet, "/controls", nil, ginctx.WithUser(t, db, user.ID))

	auth.EnforceAuth(c)

	require.False(t, c.IsAborted())
}

func TestGetUserData_LoggedInUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodGet, "/", nil, ginctx.WithUser(t, db, user.ID))

	data := auth.GetUserData(c)

	require.True(t, data.IsLoggedIn)
	require.NotEmpty(t, data.CSRFToken, "GetUserData mints a CSRF token for the session")

	// A second call on the same request/session must reuse the token
	// minted on the first call rather than minting a new one every time.
	again := auth.GetUserData(c)
	require.Equal(t, data.CSRFToken, again.CSRFToken)
}

func TestAuth_LoggedInSessionSetsPgsessionUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)
	sess := sessions.Default(c)
	sess.Set("user", user.ID)
	require.NoError(t, sess.Save())

	auth.Auth(c, db)

	require.False(t, c.IsAborted())

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
}

func TestAuthAPI_UnknownKeyReturnsForbidden(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	// A syntactically valid but unassigned API key (the column is a uuid),
	// so the lookup cleanly misses rather than failing to parse.
	c.Request.Header.Set("Authorization", "Bearer 00000000-0000-0000-0000-000000000000")

	auth.AuthAPI(c, db)

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Nil(t, pgsession.GetUser(c))
}

func TestAuthAPI_DBErrorReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	db := testDB.DB
	require.NoError(t, db.Close())

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer 00000000-0000-0000-0000-000000000000")

	auth.AuthAPI(c, db)

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuthAPI_KnownKeySetsUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	key, err := factory.APIKey(ctx, db, user.ID)
	require.NoError(t, err)

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer "+key.APIKey)

	auth.AuthAPI(c, db)

	require.False(t, c.IsAborted())
	require.NotEqual(t, http.StatusForbidden, w.Code)

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
}
