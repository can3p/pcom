package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSignupWaitingListForm_ValidateEmptyEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup_waitlist", nil)

	form := forms.SignupWaitingListFormNew(sender).(*forms.SignupWaitingListForm)
	form.Input.Email = ""

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSignupWaitingListForm_ValidateExistingUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup_waitlist", nil)

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SignupWaitingListFormNew(sender).(*forms.SignupWaitingListForm)
	form.Input.Email = existingUser.Email

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSignupWaitingListForm_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup_waitlist", nil)

	form := forms.SignupWaitingListFormNew(sender).(*forms.SignupWaitingListForm)
	form.Input.Email = "valid@example.test"

	err := form.Validate(c, db)
	require.NoError(t, err)
}

func TestSignupWaitingListForm_SaveCreatesRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupWaitingListFormNew(sender).(*forms.SignupWaitingListForm)
	form.Input.Email = "newrequest@example.test"
	form.Input.Reason = "I'm interested"
	form.Input.Attribution = "twitter"

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	exists, err := factory.SignupRequestExists(ctx, db, form.Input.Email)
	require.NoError(t, err)
	require.True(t, exists)

	sent := sender.Sent()
	require.Len(t, sent, 2)
	require.Equal(t, "waiting_list_confirm", sent[0].EmailType)
	require.Equal(t, form.Input.Email, sent[0].Mail.To[0].Address)
}

// TestSignupWaitingListForm_SaveDoesNotNormalizeEmail pins a bug: unlike
// SignupForm.Save, this Save persists the waiting-list email exactly as
// submitted. Validate normalizes (trims and lowercases) address only for its
// own duplicate check and never writes the result back to f.Input.Email, so
// a second request that differs from an already-saved one only by case or
// whitespace is not caught as a duplicate.
func TestSignupWaitingListForm_SaveDoesNotNormalizeEmail(t *testing.T) {
	t.Skip("known bug #158: SignupWaitingListForm.Save stores the raw email instead of the trimmed/lowercased value Validate checks against, so a request differing only by case or whitespace from an existing one is not rejected as a duplicate")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupWaitingListFormNew(sender).(*forms.SignupWaitingListForm)
	form.Input.Email = "  Dup@EXAMPLE.TEST  "

	_, err := form.Save(ctx, db)
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/signup_waitlist", nil)
	dup := forms.SignupWaitingListFormNew(sender).(*forms.SignupWaitingListForm)
	dup.Input.Email = "dup@example.test"

	err = dup.Validate(c, db)
	require.Error(t, err, "a request for the same address, normalized, should be rejected as a duplicate")
	require.True(t, dup.Errors.HasError("email"))
}
