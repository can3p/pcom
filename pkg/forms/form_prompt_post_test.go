package forms_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func newPromptForm(t *testing.T, db *sqlx.DB, sender *fakesender.Sender, u *model.User, directConnections []*model.User, message, recipientHandle string) *forms.PostPromptForm {
	t.Helper()
	form, ok := forms.PostPromptFormNew(postsService(db, sender), u, directConnections).(*forms.PostPromptForm)
	require.True(t, ok)
	form.Input.Message = message
	form.Input.RecipientHandle = recipientHandle

	return form
}

func TestPostPromptForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker := testutil.Must(factory.User(ctx, db))(t)
	bob := testutil.Must(factory.User(ctx, db))(t)
	carol := testutil.Must(factory.User(ctx, db))(t)
	// A prompt sent moments ago rate-limits the next one, whoever it is for.
	recentAsker := testutil.Must(factory.User(ctx, db))(t)
	testutil.Must(factory.PostPrompt(ctx, db, recentAsker.ID, bob.ID))(t)

	const msg = "Tell us about your week!"

	for _, tc := range []struct {
		name      string
		asker     *model.User
		direct    []*model.User
		message   string
		recipient *model.User
		ok        bool
	}{
		{"a normal message to a direct connection", asker, []*model.User{bob}, msg, bob, true},
		{"message too short", asker, []*model.User{bob}, "hi", bob, false},
		// Validate consults only the list it was built with, not the database.
		{"recipient outside the direct connections", asker, []*model.User{bob}, msg, carol, false},
		{"no direct connections", asker, nil, msg, bob, false},
		{"rate limited", recentAsker, []*model.User{bob, carol}, msg, carol, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, _ := newCtx(t)
			err := newPromptForm(t, db, fakesender.New(), tc.asker, tc.direct, tc.message, tc.recipient.Username).Validate(c)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPostPromptForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker := testutil.Must(factory.User(ctx, db))(t)
	recipient := testutil.Must(factory.User(ctx, db))(t)
	// the service checks the connection itself, so it has to exist
	_, _, err := factory.Connect(ctx, db, asker.ID, recipient.ID)
	require.NoError(t, err)

	sender := fakesender.New()
	form := newPromptForm(t, db, sender, asker, []*model.User{recipient}, "Tell us about your week!", recipient.Username)

	c, _ := newCtx(t)
	require.NoError(t, form.Validate(c))
	action := testutil.Must(form.Save(ctx))(t)
	action(c, form)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "post_prompt", sent[0].EmailType)
	require.Equal(t, recipient.Email, sent[0].Mail.To[0].Address)
}
