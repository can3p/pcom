package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSendInviteForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		email        func(t *testing.T, inviter *core.User) string
		wantErrField string
		skip         string
	}{
		{"empty email", func(t *testing.T, inviter *core.User) string { return "" }, "email", ""},
		{"invalid format", func(t *testing.T, inviter *core.User) string { return "not-an-email" }, "email", ""},
		{"existing user", func(t *testing.T, inviter *core.User) string {
			return testutil.Must(factory.User(ctx, db))(t).Email
		}, "email", ""},
		{"success", func(t *testing.T, inviter *core.User) string {
			testutil.Must(factory.Invitation(ctx, db, inviter.ID))(t)
			return "newinvitee@example.test"
		}, "", ""},
		{"address with a pending invitation", func(t *testing.T, inviter *core.User) string {
			testutil.Must(factory.Invitation(ctx, db, inviter.ID))(t)
			testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("pending@example.test")))(t)
			return "pending@example.test"
		}, "email", "known bug #168: the same email address can be invited more than once"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.skip != "" {
				t.Skip(tt.skip)
			}

			inviter := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

			form := forms.SendInviteFormNew(fakesender.New(), inviter).(*forms.SendInviteForm)
			form.Input.Email = tt.email(t, inviter)

			err := form.Validate(c, db)
			if tt.wantErrField == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, form.Errors.HasError(tt.wantErrField))
		})
	}
}

func TestSendInviteForm_SaveSendsInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter := testutil.Must(factory.User(ctx, db))(t)
	testutil.Must(factory.Invitation(ctx, db, inviter.ID))(t)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = "newinvitee@example.test"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "user_invitation", sent[0].EmailType)
	require.Equal(t, form.Input.Email, sent[0].Mail.To[0].Address)
}
