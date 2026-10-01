package accounts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// TestMailFailuresAreReturned: when the sender fails, the service returns its
// error, and the account change it belongs to is rolled back.
func TestMailFailuresAreReturned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	boom := errors.New("smtp down")
	failing := func() *fakesender.Sender {
		s := fakesender.New()
		s.FailWith(boom)

		return s
	}

	t.Run("register leaves no user behind", func(t *testing.T) {
		t.Parallel()

		_, err := svcWith(db, failing()).Register(ctx, "rolled-back@example.test", "rolledback", "")
		require.ErrorIs(t, err, boom)

		_, err = factory.GetUserByEmail(ctx, db, "rolled-back@example.test")
		require.Error(t, err, "the user row was rolled back with the failed mail")
	})

	t.Run("join waiting list", func(t *testing.T) {
		t.Parallel()

		err := svcWith(db, failing()).JoinWaitingList(ctx, "waiter-fails@example.test", "", "")
		require.ErrorIs(t, err, boom)

		var fatal *accounts.FatalError
		require.ErrorAs(t, err, &fatal, "a failed confirmation mail is a bug for the transport")
	})

	t.Run("send invite", func(t *testing.T) {
		t.Parallel()

		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		require.ErrorIs(t, sendInvite(ctx, db, failing(), inviter, "invitee-fails@example.test"), boom)

		// the invitation slot came back with the rollback
		require.NoError(t, sendInvite(ctx, db, fakesender.New(), inviter, "invitee-fails@example.test"))
	})
}
