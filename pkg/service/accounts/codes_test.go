package accounts_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clock is a settable time for one service.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// codeSvc is the accounts service over the real mail queue, timed by c and
// keyed with key ("" for none).
func codeSvc(db *sqlx.DB, c *clock, key string) *accounts.Service {
	queue := dbsender.NewSender(repo.New(db), fakesender.New().Delivery())
	opts := []accounts.Option{accounts.WithClock(c.now)}
	if key != "" {
		opts = append(opts, accounts.WithCodeKey(key))
	}

	return accounts.New(repo.New(db), queue, nil, opts...)
}

var codeRe = regexp.MustCompile(`login code is (\d{8})`)

// mailedCodes returns the codes queued for the address, in no order.
func mailedCodes(t *testing.T, db *sqlx.DB, to string) []string {
	t.Helper()

	var codes []string
	for _, e := range testutil.Must(factory.ListOutgoingEmails(context.Background(), db, core.OutgoingEmailWhere.EmailType.EQ("login_code")))(t) {
		var m sender.Mail
		require.NoError(t, e.Payload.Unmarshal(&m))
		if m.To[0].Address == to {
			codes = append(codes, codeRe.FindStringSubmatch(m.Text)[1])
		}
	}

	return codes
}

// start starts a login for the user's address and returns the attempt and
// the one code it mailed.
func start(t *testing.T, ctx context.Context, db *sqlx.DB, svc *accounts.Service, u *core.User, returnURL string) (string, string) {
	t.Helper()

	before := mailedCodes(t, db, u.Email)
	id := testutil.Must(svc.StartLogin(ctx, u.Email, returnURL))(t)
	after := mailedCodes(t, db, u.Email)
	require.Len(t, after, len(before)+1)

	for _, c := range before {
		i := slices.Index(after, c)
		after = slices.Delete(after, i, i+1)
	}

	return id, after[0]
}

// signupConfirmed counts the admin's "confirmed email" notices naming email.
func signupConfirmed(t *testing.T, db *sqlx.DB, email string) int {
	t.Helper()

	n := 0
	for _, e := range testutil.Must(factory.ListOutgoingEmails(context.Background(), db, core.OutgoingEmailWhere.EmailType.EQ("signup_confirmed")))(t) {
		var m sender.Mail
		require.NoError(t, e.Payload.Unmarshal(&m))
		if strings.Contains(m.Text, email) {
			n++
		}
	}

	return n
}

func otherCode(code string) string {
	n, _ := strconv.Atoi(code)

	return fmt.Sprintf("%08d", (n+1)%100_000_000)
}

func requireWrongCode(t *testing.T, err error) {
	t.Helper()

	var invalid *service.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, "code", invalid.Field)
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestLoginCodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	newUser := func(t *testing.T) *core.User { return testutil.Must(factory.User(ctx, db))(t) }

	t.Run("a confirmed user's code logs in once, with the address lowercased", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		u := newUser(t)

		id := testutil.Must(svc.StartLogin(ctx, " "+strings.ToUpper(u.Email)+" ", "/posts/1"))(t)
		codes := mailedCodes(t, db, u.Email)
		require.Len(t, codes, 1)

		got, returnURL, err := svc.FinishLogin(ctx, id, codes[0])
		require.NoError(t, err)
		require.Equal(t, u.ID, got.ID)
		require.Equal(t, "/posts/1", returnURL)

		_, _, err = svc.FinishLogin(ctx, id, codes[0])
		requireNotFound(t, err)
	})

	t.Run("a signup attempt is finished with the mailed code, which confirms the email", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		email := "signup-code@example.test"

		id := testutil.Must(svc.Register(ctx, email, "signupcode", ""))(t)
		require.Empty(t, mailedCodes(t, db, email), "the signup mail is not a login code")

		// the address is not confirmed: no login code is mailed, and the attempt
		// StartLogin made never logs in
		other := testutil.Must(svc.StartLogin(ctx, email, ""))(t)
		require.Empty(t, mailedCodes(t, db, email))
		_, _, err := svc.FinishLogin(ctx, other, "00000000")
		requireWrongCode(t, err)

		before := signupConfirmed(t, db, email)

		var code string
		for _, e := range testutil.Must(factory.ListOutgoingEmails(ctx, db, core.OutgoingEmailWhere.EmailType.EQ("confirm_signup")))(t) {
			var m sender.Mail
			require.NoError(t, e.Payload.Unmarshal(&m))
			if m.To[0].Address == email {
				code = regexp.MustCompile(`confirmation code is (\d{8})`).FindStringSubmatch(m.Text)[1]
			}
		}
		require.NotEmpty(t, code)

		_, _, err = svc.FinishLogin(ctx, id, otherCode(code))
		requireWrongCode(t, err)
		require.False(t, testutil.Must(factory.GetUserByEmail(ctx, db, email))(t).EmailConfirmedAt.Valid)
		require.Equal(t, before, signupConfirmed(t, db, email))

		got, _, err := svc.FinishLogin(ctx, id, code)
		require.NoError(t, err)
		require.Equal(t, "signupcode", got.Username)
		require.True(t, testutil.Must(factory.GetUserByEmail(ctx, db, email))(t).EmailConfirmedAt.Valid)
		require.Equal(t, before+1, signupConfirmed(t, db, email), "the admin hears of the confirmation once")
	})

	t.Run("an accepted invitation starts an attempt for the new user", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		inviter := newUser(t)
		invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee-code@example.test")))(t)

		id := testutil.Must(svc.AcceptInvite(ctx, invite, "inviteecode"))(t)
		codes := mailedCodes(t, db, "invitee-code@example.test")
		require.Len(t, codes, 1)

		got, _, err := svc.FinishLogin(ctx, id, codes[0])
		require.NoError(t, err)
		require.Equal(t, invite.CreatedUserID.String, got.ID)
		require.Zero(t, signupConfirmed(t, db, "invitee-code@example.test"), "an invited address was confirmed from the start")
	})

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, svc *accounts.Service) (email string, mailedBefore int)
	}{
		{"unknown address", func(t *testing.T, _ *accounts.Service) (string, int) {
			return "nobody-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", 0
		}},
		{"unconfirmed user", func(t *testing.T, _ *accounts.Service) (string, int) {
			return testutil.Must(factory.User(ctx, db, factory.Unconfirmed()))(t).Email, 0
		}},
		{"user over the limit", func(t *testing.T, svc *accounts.Service) (string, int) {
			u := newUser(t)
			for range 3 {
				testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
			}

			return u.Email, 3
		}},
	} {
		// a stranger cannot tell any of these from a real login: every try is
		// a wrong code until the attempt is out of tries
		t.Run(tc.name+" is mailed nothing and answers like a wrong code", func(t *testing.T) {
			t.Parallel()

			svc := codeSvc(db, &clock{time.Now()}, "test-key")
			email, before := tc.setup(t, svc)

			id := testutil.Must(svc.StartLogin(ctx, email, ""))(t)
			require.NotEmpty(t, id)
			require.Len(t, mailedCodes(t, db, email), before)

			for range 5 {
				_, _, err := svc.FinishLogin(ctx, id, "00000000")
				requireWrongCode(t, err)
			}

			_, _, err := svc.FinishLogin(ctx, id, "00000000")
			requireNotFound(t, err)
		})
	}

	t.Run("concurrent logins mail no more codes than the limit", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		u := newUser(t)

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				_, err := svc.StartLogin(ctx, u.Email, "")
				assert.NoError(t, err)
			})
		}
		wg.Wait()

		require.Len(t, mailedCodes(t, db, u.Email), 3)
	})

	t.Run("after too many wrong codes the user cannot log in or get a code until the window passes", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		t0 := c.t
		svc := codeSvc(db, c, "test-key")
		u := newUser(t)
		a, codeA := start(t, ctx, db, svc, u, "")
		b, codeB := start(t, ctx, db, svc, u, "")
		last, lastCode := start(t, ctx, db, svc, u, "")

		for _, at := range []struct{ id, code string }{{a, codeA}, {b, codeB}} {
			for range 5 {
				_, _, err := svc.FinishLogin(ctx, at.id, otherCode(at.code))
				requireWrongCode(t, err)
			}
		}

		_, _, err := svc.FinishLogin(ctx, last, lastCode)
		requireWrongCode(t, err)

		// the mail limit is free again, the wrong codes still count
		c.t = t0.Add(16 * time.Minute)
		testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		require.Len(t, mailedCodes(t, db, u.Email), 3)

		c.t = t0.Add(time.Hour + time.Second)
		id, code := start(t, ctx, db, svc, u, "")
		got, _, err := svc.FinishLogin(ctx, id, code)
		require.NoError(t, err)
		require.Equal(t, u.ID, got.ID)
	})

	t.Run("pruning deletes only attempts no limit counts any more", func(t *testing.T) {
		t.Parallel()

		// pruning sweeps the whole table, so it gets a database of its own
		db := testdb.New(t).DB
		c := &clock{time.Now()}
		t0 := c.t
		svc := codeSvc(db, c, "test-key")
		u := testutil.Must(factory.User(ctx, db))(t)
		old, _ := start(t, ctx, db, svc, u, "")
		c.t = t0.Add(10 * time.Minute)
		recent, _ := start(t, ctx, db, svc, u, "")

		// old expired at t0+15m, recent at t0+25m; the cut is an hour before now
		c.t = t0.Add(time.Hour + 20*time.Minute)
		testutil.Must(svc.PruneLoginAttempts(ctx))(t)

		_, err := repo.New(db).LockLoginAttempt(ctx, old)
		require.ErrorIs(t, err, repo.ErrNotFound)
		testutil.Must(repo.New(db).LockLoginAttempt(ctx, recent))(t)
	})

	t.Run("the limit is per user and frees up after its window", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		t0 := c.t
		svc := codeSvc(db, c, "test-key")
		u, other := newUser(t), newUser(t)
		for range 4 {
			testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		}
		require.Len(t, mailedCodes(t, db, u.Email), 3)

		testutil.Must(svc.StartLogin(ctx, other.Email, ""))(t)
		require.Len(t, mailedCodes(t, db, other.Email), 1)

		c.t = t0.Add(15*time.Minute - time.Second)
		testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		require.Len(t, mailedCodes(t, db, u.Email), 3)

		c.t = t0.Add(15*time.Minute + time.Second)
		testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		require.Len(t, mailedCodes(t, db, u.Email), 4)
	})

	t.Run("wrong codes are counted and the sixth try fails even with the right code", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		u := newUser(t)
		id, code := start(t, ctx, db, svc, u, "")

		for range 5 {
			_, _, err := svc.FinishLogin(ctx, id, otherCode(code))
			requireWrongCode(t, err)
		}

		_, _, err := svc.FinishLogin(ctx, id, code)
		requireNotFound(t, err)
		_, err = svc.IssueLoginCode(ctx, id)
		requireNotFound(t, err)
		_, err = svc.LatestLoginAttempt(ctx, u.Email)
		requireNotFound(t, err)
	})

	t.Run("an attempt works until it expires", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		t0 := c.t
		svc := codeSvc(db, c, "test-key")
		u := newUser(t)
		early, earlyCode := start(t, ctx, db, svc, u, "")
		late, lateCode := start(t, ctx, db, svc, u, "")

		c.t = t0.Add(15*time.Minute - time.Second)
		_, _, err := svc.FinishLogin(ctx, early, earlyCode)
		require.NoError(t, err)

		c.t = t0.Add(15 * time.Minute)
		_, _, err = svc.FinishLogin(ctx, late, lateCode)
		requireNotFound(t, err)
		_, err = svc.IssueLoginCode(ctx, late)
		requireNotFound(t, err)
		_, err = svc.LatestLoginAttempt(ctx, u.Email)
		requireNotFound(t, err)
	})

	t.Run("an issued code replaces the mailed one on the latest open attempt", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		svc := codeSvc(db, c, "test-key")
		u := newUser(t)
		older, _ := start(t, ctx, db, svc, u, "")
		c.advance(time.Second)
		newer, mailed := start(t, ctx, db, svc, u, "/feed")

		latest := testutil.Must(svc.LatestLoginAttempt(ctx, strings.ToUpper(u.Email)))(t)
		require.Equal(t, newer, latest)

		code := testutil.Must(svc.IssueLoginCode(ctx, latest))(t)
		_, _, err := svc.FinishLogin(ctx, newer, mailed)
		requireWrongCode(t, err)

		got, returnURL, err := svc.FinishLogin(ctx, newer, code)
		require.NoError(t, err)
		require.Equal(t, u.ID, got.ID)
		require.Equal(t, "/feed", returnURL)

		require.Equal(t, older, testutil.Must(svc.LatestLoginAttempt(ctx, u.Email))(t))
		_, err = svc.IssueLoginCode(ctx, newer)
		requireNotFound(t, err)
		_, err = svc.LatestLoginAttempt(ctx, "nobody@example.test")
		requireNotFound(t, err)
	})

	t.Run("a code works only on its own attempt", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		u := newUser(t)
		_, codeA := start(t, ctx, db, svc, u, "")
		b, codeB := start(t, ctx, db, svc, u, "")

		_, _, err := svc.FinishLogin(ctx, b, codeA)
		requireWrongCode(t, err)
		_, _, err = svc.FinishLogin(ctx, b, codeB)
		require.NoError(t, err)
	})

	t.Run("the code is stored keyed, and another key rejects it", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		svc := codeSvc(db, c, "test-key")
		u := newUser(t)
		id, code := start(t, ctx, db, svc, u, "")

		stored := testutil.Must(repo.New(db).LockLoginAttempt(ctx, id))(t)
		require.True(t, stored.CodeHash.Valid)
		require.NotContains(t, stored.CodeHash.String, code)

		_, _, err := codeSvc(db, c, "other-key").FinishLogin(ctx, id, code)
		requireWrongCode(t, err)
		_, _, err = svc.FinishLogin(ctx, id, code)
		require.NoError(t, err)
	})

	t.Run("a malformed or unknown attempt id is not found", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()}, "test-key")
		for _, id := range []string{"not-an-id", "00000000-0000-0000-0000-000000000000"} {
			_, _, err := svc.FinishLogin(ctx, id, "12345678")
			requireNotFound(t, err)
			_, err = svc.IssueLoginCode(ctx, id)
			requireNotFound(t, err)
		}
	})

	t.Run("without a code key nothing is issued or checked", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		keyed, unkeyed := codeSvc(db, c, "test-key"), codeSvc(db, c, "")
		u := newUser(t)

		_, err := unkeyed.StartLogin(ctx, u.Email, "")
		require.Error(t, err)
		require.Empty(t, mailedCodes(t, db, u.Email))
		_, err = keyed.LatestLoginAttempt(ctx, u.Email)
		requireNotFound(t, err)

		id, code := start(t, ctx, db, keyed, u, "")
		_, _, err = unkeyed.FinishLogin(ctx, id, code)
		require.Error(t, err)
		require.NotErrorIs(t, err, service.ErrNotFound)
		_, err = unkeyed.IssueLoginCode(ctx, id)
		require.Error(t, err)
		require.NotErrorIs(t, err, service.ErrNotFound)
		_, err = unkeyed.LatestLoginAttempt(ctx, u.Email)
		require.Error(t, err)
		require.NotErrorIs(t, err, service.ErrNotFound)

		_, _, err = keyed.FinishLogin(ctx, id, code)
		require.NoError(t, err)
	})
}
