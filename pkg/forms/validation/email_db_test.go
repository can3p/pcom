package validation_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestEmailOKToSignup(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	tests := []struct {
		name   string
		email  func(t *testing.T) string
		wantOK bool
	}{
		{"existing user", func(t *testing.T) string { return testutil.Must(factory.User(ctx, db))(t).Email }, false},
		{"plus sign", func(t *testing.T) string { return "user+test@example.com" }, false},
		{"plus sign allowed test email", func(t *testing.T) string { return "dpetroff+test@gmail.com" }, true},
		{"disposable domain", func(t *testing.T) string { return "test@mailinator.com" }, false},
		{"valid email", func(t *testing.T) string { return "valid@gmail.com" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, isOK := validation.EmailOKToSignup(ctx, db, sender, tt.email(t))
			require.Equal(t, tt.wantOK, isOK)
		})
	}
}

func TestEmailOKToAddToWaitingList(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name   string
		email  func(t *testing.T) string
		wantOK bool
	}{
		{"existing user", func(t *testing.T) string { return testutil.Must(factory.User(ctx, db))(t).Email }, false},
		{"existing invitation", func(t *testing.T) string {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("existing@example.test")))(t)
			return "existing@example.test"
		}, false},
		{"existing invitation, spelled differently", func(t *testing.T) string {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("spelled@example.test")))(t)
			return " Spelled@Example.test "
		}, false},
		{"existing signup request", func(t *testing.T) string {
			return testutil.Must(factory.SignupRequest(ctx, db))(t).Email
		}, false},
		{"valid email", func(t *testing.T) string { return "valid@example.test" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, isOK := validation.EmailOKToAddToWaitingList(ctx, db, tt.email(t))
			require.Equal(t, tt.wantOK, isOK)
		})
	}
}
