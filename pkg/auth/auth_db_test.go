package auth_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

func TestSignup_InsertsUnconfirmedUserAndNotifiesAdmin(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	u, err := auth.Signup(ctx, db, sender, "new-signup@example.test", "newsignup", "s3cr3t-pw", "some-campaign")
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)

	got := testutil.Must(factory.GetUser(ctx, db, u.ID))(t)
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

	existing := testutil.Must(factory.User(ctx, db))(t)

	_, err := auth.Signup(ctx, db, sender, existing.Email, "someoneelse", "s3cr3t-pw", "")
	require.Error(t, err, "the email column is unique, so a second signup for it must fail")
	require.Empty(t, sender.Sent(), "a failed signup must not notify anyone")
}

func TestAcceptInvite_CreatesUserAndConnectsToInviter(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee@example.test")))(t)

	require.NoError(t, auth.AcceptInvite(ctx, db, sender, invite, "invitee", "s3cr3t-pw"))

	// AcceptInvite mutates the invite it was given: CreatedUserID is set to
	// the freshly inserted user's ID.
	require.True(t, invite.CreatedUserID.Valid)
	newUserID := invite.CreatedUserID.String

	gotUser := testutil.Must(factory.GetUser(ctx, db, newUserID))(t)
	require.Equal(t, "invitee", gotUser.Username)
	require.Equal(t, "invitee@example.test", gotUser.Email)
	require.True(t, gotUser.EmailConfirmedAt.Valid, "accepting an invite confirms the email right away")
	require.Equal(t, "accepted_invite", gotUser.SignupAttribution.String)

	require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, inviter.ID, newUserID))(t), "accepting an invite connects the new user to the inviter")

	require.Len(t, sender.Sent(), 1, "accepting an invite notifies the admin of the new user")
}

func TestAcceptInvite_MissingFieldsReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee2@example.test")))(t)

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

	inviter := testutil.Must(factory.User(ctx, db))(t)
	existing := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent(existing.Email)))(t)

	err := auth.AcceptInvite(ctx, db, sender, invite, "someoneelse", "s3cr3t-pw")
	require.Error(t, err, "the email column is unique, so accepting into an already-used email must fail")
	require.False(t, invite.CreatedUserID.Valid)
	require.Empty(t, sender.Sent())
}

func TestCheckCredentials(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("correct-horse")))(t)

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

func TestLogin(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("s3cr3t-pw")))(t)

	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "sets session on success", password: "s3cr3t-pw"},
		{name: "bad credentials returns error without setting session", password: "wrong-pw", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

			err := auth.Login(c, db, user.Email, tc.password)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, sessions.Default(c).Get("user"))
			} else {
				require.NoError(t, err)
				require.Equal(t, user.ID, sessions.Default(c).Get("user"))
			}
		})
	}
}

// TestLogin_EmailCaseAndLegacyHashes pins #114 and #119. The email is
// matched in any case. A legacy sha256 hash was computed from the email as
// stored, so it is checked against that spelling, then replaced by argon2id
// and the email is normalized, unless another account shares the
// normalized email.
func TestLogin_EmailCaseAndLegacyHashes(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	type account struct{ email, password, typed, wantEmail string }

	cases := []struct {
		name     string
		accounts []account
	}{
		{name: "mixed-case invitee types lower case", accounts: []account{
			{email: "Alice@X.test", password: "pw-a", typed: "alice@x.test", wantEmail: "alice@x.test"},
		}},
		{name: "mixed-case invitee types upper case", accounts: []account{
			{email: "Carol@X.test", password: "pw-c", typed: "CAROL@X.TEST", wantEmail: "carol@x.test"},
		}},
		{name: "lowercase signup types upper case", accounts: []account{
			{email: "dave@x.test", password: "pw-d", typed: " DAVE@x.test", wantEmail: "dave@x.test"},
		}},
		{name: "colliding legacy accounts log in with their own password", accounts: []account{
			{email: "Bob@x.test", password: "pw-1", typed: "bob@x.test", wantEmail: "Bob@x.test"},
			{email: "BOB@x.test", password: "pw-2", typed: "bob@x.test", wantEmail: "BOB@x.test"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ids := make([]string, len(tc.accounts))
			for i, a := range tc.accounts {
				ids[i] = testutil.Must(factory.User(ctx, db, factory.WithEmail(a.email), factory.WithPassword(a.password)))(t).ID
			}

			for i, a := range tc.accounts {
				c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
				require.NoError(t, auth.Login(c, db, a.typed, a.password))
				require.Equal(t, ids[i], sessions.Default(c).Get("user"))

				got := testutil.Must(factory.GetUser(ctx, db, ids[i]))(t)
				require.True(t, strings.HasPrefix(got.Pwdhash.String, "$argon2id$"), "a legacy hash is replaced at login")
				require.Equal(t, a.wantEmail, got.Email)

				c2, _ := ginctx.New(t, http.MethodPost, "/login", nil)
				require.NoError(t, auth.Login(c2, db, a.typed, a.password), "the new hash logs in too")
				require.Equal(t, ids[i], sessions.Default(c2).Get("user"))
				require.Error(t, auth.Login(c2, db, a.typed, "wrong-pw"))
			}
		})
	}
}

func TestAcceptInvite_MixedCaseEmailLogsInLowercase(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("Bob@Example.test")))(t)

	require.NoError(t, auth.AcceptInvite(ctx, db, fakesender.New(), invite, "bob", "s3cr3t-pw"))

	got := testutil.Must(factory.GetUser(ctx, db, invite.CreatedUserID.String))(t)
	require.Equal(t, "bob@example.test", got.Email)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
	require.NoError(t, auth.Login(c, db, "bob@example.test", "s3cr3t-pw"))
}

func TestLogin_DBErrorIsReturnedAsIs(t *testing.T) {
	t.Parallel()

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

	user := testutil.Must(factory.User(ctx, db))(t)

	c, _ := ginctx.New(t, http.MethodGet, "/controls", nil, ginctx.WithUser(t, db, user.ID))

	auth.EnforceAuth(c)

	require.False(t, c.IsAborted())
}

func TestGetUserData_LoggedInUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

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

	user := testutil.Must(factory.User(ctx, db))(t)

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

	user := testutil.Must(factory.User(ctx, db))(t)

	key := testutil.Must(factory.APIKey(ctx, db, user.ID))(t)

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer "+key.APIKey)

	auth.AuthAPI(c, db)

	require.False(t, c.IsAborted())
	require.NotEqual(t, http.StatusForbidden, w.Code)

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
}

func TestLogin_AccountWithoutPasswordIsSkipped(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	noPassword := func(u *core.User) { u.Pwdhash = null.String{} }

	cases := []struct {
		name     string
		withReal bool
		wantErr  bool
	}{
		{name: "real account sharing the email wins", withReal: true},
		{name: "account without a password alone cannot log in", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			email := strings.ReplaceAll(tc.name, " ", "-") + "@x.test"
			// created first, so it is tried first
			testutil.Must(factory.User(ctx, db, factory.WithEmail(strings.ToUpper(email)), noPassword))(t)

			var realID string
			if tc.withReal {
				realID = testutil.Must(factory.User(ctx, db, factory.WithEmail(email), factory.WithPassword("real-pw")))(t).ID
			}

			c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
			err := auth.Login(c, db, email, "real-pw")

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, realID, sessions.Default(c).Get("user"))
		})
	}
}

func TestSignupAndAcceptInvite_ReturnAdminNotificationError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	boom := errors.New("smtp down")

	cases := []struct {
		name string
		run  func(s *fakesender.Sender) error
	}{
		{name: "signup", run: func(s *fakesender.Sender) error {
			_, err := auth.Signup(ctx, db, s, "fail-signup@x.test", "failsignup", "s3cr3t-pw", "")
			return err
		}},
		{name: "accept invite", run: func(s *fakesender.Sender) error {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("fail-invitee@x.test")))(t)

			return auth.AcceptInvite(ctx, db, s, invite, "failinvitee", "s3cr3t-pw")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			s.FailWith(boom)

			require.ErrorIs(t, tc.run(s), boom)
		})
	}
}
