package mail_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// newUser wraps the factory call this file repeats.
func newUser(t *testing.T, ctx context.Context, db *sqlx.DB) *model.User {
	t.Helper()

	return testutil.Must(factory.User(ctx, db))(t)
}

func TestPendingInvitationUniqueIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	inviter := newUser(t, ctx, db)

	t.Run("a second pending invitation to the same address fails", func(t *testing.T) {
		t.Parallel()

		testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-dup@example.test")))(t)
		_, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-dup@example.test"))
		require.Error(t, err)
	})

	t.Run("the address is free again once the first invitation is used", func(t *testing.T) {
		t.Parallel()

		used := testutil.Must(factory.User(ctx, db))(t)
		testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-used@example.test"), factory.UsedBy(used.ID)))(t)
		_, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-used@example.test"))
		require.NoError(t, err)
	})
}

// TestEmailConstraints: every stored email is normalized (lowercase, no
// surrounding space) and has one "@" with text on both sides, so lookups can
// compare by plain equality; users.email is unique.
func TestEmailConstraints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	inviter := newUser(t, ctx, db)

	insert := map[string]func(email string) error{
		"user": func(email string) error {
			_, err := factory.User(ctx, db, factory.WithEmail(email))
			return err
		},
		"invitation": func(email string) error {
			_, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent(email))
			return err
		},
		"signup request": func(email string) error {
			_, err := factory.SignupRequest(ctx, db, func(r *model.UserSignupRequest) { r.Email = email })
			return err
		},
	}

	bad := []string{"Mixed@example.test", " space@example.test", "space@example.test ", "no-at.example.test", "two@at@example.test", "@example.test", "nobody@"}

	for table, insert := range insert {
		for i, email := range bad {
			require.Error(t, insert(email), "%s %q", table, email)
			require.NoError(t, insert(fmt.Sprintf("ok-%d@%s.test", i, strings.ReplaceAll(table, " ", "-"))), table)
		}
	}

	testutil.Must(factory.User(ctx, db, factory.WithEmail("taken@example.test")))(t)
	_, err := factory.User(ctx, db, factory.WithEmail("taken@example.test"))
	require.Error(t, err, "users.email is unique")
}
