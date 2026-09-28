package validation_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestEmailOKToSignup_ExistingUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	sender := fakesender.New()
	_, isOK := validation.EmailOKToSignup(ctx, db, sender, ofExistingUser.Email)
	require.False(t, isOK)
}

func TestEmailOKToSignup_PlusSign(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	sender := fakesender.New()
	_, isOK := validation.EmailOKToSignup(ctx, db, sender, "user+test@example.com")
	require.False(t, isOK)
}

func TestEmailOKToSignup_PlusSignAllowedTestEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	sender := fakesender.New()
	_, isOK := validation.EmailOKToSignup(ctx, db, sender, "dpetroff+test@gmail.com")
	require.True(t, isOK)
}

func TestEmailOKToSignup_DisposableDomain(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	sender := fakesender.New()
	_, isOK := validation.EmailOKToSignup(ctx, db, sender, "test@mailinator.com")
	require.False(t, isOK)
}

func TestEmailOKToSignup_ValidEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	sender := fakesender.New()
	_, isOK := validation.EmailOKToSignup(ctx, db, sender, "valid@gmail.com")
	require.True(t, isOK)
}

func TestEmailOKToAddToWaitingList_ExistingUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, isOK := validation.EmailOKToAddToWaitingList(ctx, db, ofExistingUser.Email)
	require.False(t, isOK)
}

func TestEmailOKToAddToWaitingList_ExistingInvitation(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofInviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Invitation(ctx, db, ofInviter.ID, factory.Sent("existing@example.test"))
	require.NoError(t, err)

	_, isOK := validation.EmailOKToAddToWaitingList(ctx, db, "existing@example.test")
	require.False(t, isOK)
}

func TestEmailOKToAddToWaitingList_ExistingSignupRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingRequest, err := factory.SignupRequest(ctx, db)
	require.NoError(t, err)

	_, isOK := validation.EmailOKToAddToWaitingList(ctx, db, ofExistingRequest.Email)
	require.False(t, isOK)
}

func TestEmailOKToAddToWaitingList_ValidEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	_, isOK := validation.EmailOKToAddToWaitingList(ctx, db, "valid@example.test")
	require.True(t, isOK)
}
