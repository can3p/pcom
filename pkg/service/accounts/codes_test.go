package accounts_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	"github.com/stretchr/testify/require"
)

// clock is a settable time for one service.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// codeSvc is the accounts service over the real mail queue, timed by c.
func codeSvc(db *sqlx.DB, c *clock) *accounts.Service {
	queue := dbsender.NewSender(repo.New(db), fakesender.New().Delivery())

	return accounts.New(repo.New(db), queue, nil, accounts.WithCodeKey("test-key"), accounts.WithClock(c.now))
}

var codeRe = regexp.MustCompile(`login code is (\d{6})`)

// mailedCodes returns the codes queued for the address, oldest first.
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

func otherCode(code string) string {
	n, _ := strconv.Atoi(code)

	return fmt.Sprintf("%06d", (n+1)%1_000_000)
}

func requireWrongCode(t *testing.T, err error) {
	t.Helper()

	var invalid *service.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, "code", invalid.Field)
}

func TestLoginCodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB

	t.Run("a confirmed user's code logs in once, with the address lowercased", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()})
		u := testutil.Must(factory.User(ctx, db))(t)

		id := testutil.Must(svc.StartLogin(ctx, " "+strings.ToUpper(u.Email)+" ", "/posts/1"))(t)
		codes := mailedCodes(t, db, u.Email)
		require.Len(t, codes, 1)

		got, returnURL, err := svc.FinishLogin(ctx, id, codes[0])
		require.NoError(t, err)
		require.Equal(t, u.ID, got.ID)
		require.Equal(t, "/posts/1", returnURL)

		_, _, err = svc.FinishLogin(ctx, id, codes[0])
		require.ErrorIs(t, err, service.ErrNotFound)
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
		{"over the limit", func(t *testing.T, svc *accounts.Service) (string, int) {
			u := testutil.Must(factory.User(ctx, db))(t)
			for range 3 {
				testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
			}

			return u.Email, 3
		}},
	} {
		t.Run(tc.name+" is mailed nothing and never logs in", func(t *testing.T) {
			t.Parallel()

			svc := codeSvc(db, &clock{time.Now()})
			email, before := tc.setup(t, svc)

			id := testutil.Must(svc.StartLogin(ctx, email, ""))(t)
			require.NotEmpty(t, id)
			require.Len(t, mailedCodes(t, db, email), before)

			_, _, err := svc.FinishLogin(ctx, id, "000000")
			require.Error(t, err)
			for _, code := range mailedCodes(t, db, email) {
				_, _, err := svc.FinishLogin(ctx, id, code)
				require.Error(t, err)
			}
		})
	}

	t.Run("the limit frees up after its window", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		svc := codeSvc(db, c)
		u := testutil.Must(factory.User(ctx, db))(t)
		for range 4 {
			testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		}
		require.Len(t, mailedCodes(t, db, u.Email), 3)

		c.advance(16 * time.Minute)
		testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		require.Len(t, mailedCodes(t, db, u.Email), 4)
	})

	t.Run("wrong codes are counted and the sixth try fails even with the right code", func(t *testing.T) {
		t.Parallel()

		svc := codeSvc(db, &clock{time.Now()})
		u := testutil.Must(factory.User(ctx, db))(t)
		id := testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		code := mailedCodes(t, db, u.Email)[0]

		for range 5 {
			_, _, err := svc.FinishLogin(ctx, id, otherCode(code))
			requireWrongCode(t, err)
		}

		_, _, err := svc.FinishLogin(ctx, id, code)
		require.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("an expired attempt fails", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		svc := codeSvc(db, c)
		u := testutil.Must(factory.User(ctx, db))(t)
		id := testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)

		c.advance(accounts.CodeLifetime + time.Second)
		_, _, err := svc.FinishLogin(ctx, id, mailedCodes(t, db, u.Email)[0])
		require.ErrorIs(t, err, service.ErrNotFound)

		_, err = svc.LatestLoginAttempt(ctx, u.Email)
		require.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("an issued code logs in to the latest open attempt", func(t *testing.T) {
		t.Parallel()

		c := &clock{time.Now()}
		svc := codeSvc(db, c)
		u := testutil.Must(factory.User(ctx, db))(t)
		older := testutil.Must(svc.StartLogin(ctx, u.Email, ""))(t)
		c.advance(time.Second)
		newer := testutil.Must(svc.StartLogin(ctx, u.Email, "/feed"))(t)

		latest := testutil.Must(svc.LatestLoginAttempt(ctx, strings.ToUpper(u.Email)))(t)
		require.Equal(t, newer, latest)

		code := testutil.Must(svc.IssueLoginCode(ctx, latest))(t)
		_, _, err := svc.FinishLogin(ctx, newer, mailedCodes(t, db, u.Email)[1])
		requireWrongCode(t, err)

		got, returnURL, err := svc.FinishLogin(ctx, latest, code)
		require.NoError(t, err)
		require.Equal(t, u.ID, got.ID)
		require.Equal(t, "/feed", returnURL)

		require.Equal(t, older, testutil.Must(svc.LatestLoginAttempt(ctx, u.Email))(t))

		_, err = svc.IssueLoginCode(ctx, newer)
		require.ErrorIs(t, err, service.ErrNotFound)
		_, err = svc.LatestLoginAttempt(ctx, "nobody@example.test")
		require.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("without a code key nothing is issued or checked", func(t *testing.T) {
		t.Parallel()

		svc := accounts.New(repo.New(db), fakesender.New(), nil)
		_, err := svc.StartLogin(ctx, "someone@example.test", "")
		require.Error(t, err)
		_, _, err = svc.FinishLogin(ctx, "00000000-0000-0000-0000-000000000000", "123456")
		require.Error(t, err)
		require.False(t, errors.Is(err, service.ErrNotFound))
		_, err = svc.IssueLoginCode(ctx, "00000000-0000-0000-0000-000000000000")
		require.Error(t, err)
	})
}
