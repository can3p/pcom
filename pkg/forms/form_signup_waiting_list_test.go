package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSignupWaitingListForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		email        func(t *testing.T) string
		wantErrField string
	}{
		{"empty email", func(t *testing.T) string { return "" }, "email"},
		{"existing user", func(t *testing.T) string { return testutil.Must(factory.User(ctx, db))(t).Email }, "email"},
		{"success", func(t *testing.T) string { return "valid@example.test" }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, _ := ginctx.New(t, http.MethodPost, "/signup_waitlist", nil)
			form := forms.SignupWaitingListFormNew(accountsFor(db, fakesender.New())).(*forms.SignupWaitingListForm)
			form.Input.Email = tt.email(t)

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

func TestSignupWaitingListForm_SaveCreatesRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupWaitingListFormNew(accountsFor(db, sender)).(*forms.SignupWaitingListForm)
	form.Input.Email = "newrequest@example.test"
	form.Input.Reason = "I'm interested"
	form.Input.Attribution = "twitter"

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	exists := testutil.Must(factory.SignupRequestExists(ctx, db, form.Input.Email))(t)
	require.True(t, exists)

	sent := sender.Sent()
	require.Len(t, sent, 2)
	require.Equal(t, "waiting_list_confirm", sent[0].EmailType)
	require.Equal(t, form.Input.Email, sent[0].Mail.To[0].Address)
}

// TestSignupWaitingListForm_SaveNormalizesEmail checks that Save stores the
// trimmed, lowercased address, so a later request differing only by case or
// whitespace is rejected as a duplicate.
func TestSignupWaitingListForm_SaveNormalizesEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupWaitingListFormNew(accountsFor(db, sender)).(*forms.SignupWaitingListForm)
	form.Input.Email = "  Dup@EXAMPLE.TEST  "

	_, err := form.Save(ctx, db)
	require.NoError(t, err)
	require.True(t, testutil.Must(factory.SignupRequestExists(ctx, db, "dup@example.test"))(t), "the normalized address is what gets stored")

	c, _ := ginctx.New(t, http.MethodPost, "/signup_waitlist", nil)
	dup := forms.SignupWaitingListFormNew(accountsFor(db, sender)).(*forms.SignupWaitingListForm)
	dup.Input.Email = "dup@example.test"

	err = dup.Validate(c, db)
	require.Error(t, err, "a request for the same address, normalized, should be rejected as a duplicate")
	require.True(t, dup.Errors.HasError("email"))
}
