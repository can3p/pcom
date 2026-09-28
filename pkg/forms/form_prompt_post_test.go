package forms_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func pfPromptForm(t *testing.T, sender *fakesender.Sender, u *core.User, directConnections []*core.User, message, recipientHandle string) *forms.PostPromptForm {
	t.Helper()
	f := forms.PostPromptFormNew(sender, u, directConnections)
	pf, ok := f.(*forms.PostPromptForm)
	require.True(t, ok)
	pf.Input.Message = message
	pf.Input.RecipientHandle = recipientHandle

	return pf
}

func TestPostPromptForm_Validate_MessageLength(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	bob, err := factory.User(ctx, db)
	require.NoError(t, err)

	t.Run("too short", func(t *testing.T) {
		t.Parallel()

		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker, []*core.User{bob}, "hi", bob.Username)
		require.Error(t, form.Validate(c, db))
	})

	t.Run("a normal message to a direct connection passes", func(t *testing.T) {
		t.Parallel()

		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker, []*core.User{bob}, "Tell us about your week!", bob.Username)
		require.NoError(t, form.Validate(c, db))
	})
}

func TestPostPromptForm_Validate_DirectOnlyRecipients(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	bob, err := factory.User(ctx, db)
	require.NoError(t, err)
	carol, err := factory.User(ctx, db)
	require.NoError(t, err)

	t.Run("a direct connection is a valid recipient", func(t *testing.T) {
		t.Parallel()

		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker, []*core.User{bob}, "Tell us about your week!", bob.Username)
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("a handle outside the direct connections list is rejected", func(t *testing.T) {
		t.Parallel()

		// carol exists in the database but was not passed in as one of
		// asker's direct connections: Validate only ever consults the
		// list it was constructed with.
		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker, []*core.User{bob}, "Tell us about your week!", carol.Username)
		require.Error(t, form.Validate(c, db))
	})

	t.Run("an empty direct connections list rejects everyone", func(t *testing.T) {
		t.Parallel()

		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker, nil, "Tell us about your week!", bob.Username)
		require.Error(t, form.Validate(c, db))
	})
}

func TestPostPromptForm_Validate_RateLimit(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	bob, err := factory.User(ctx, db)
	require.NoError(t, err)
	carol, err := factory.User(ctx, db)
	require.NoError(t, err)

	t.Run("no rate limit before the asker has ever prompted", func(t *testing.T) {
		t.Parallel()

		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker, []*core.User{bob}, "Tell us about your week!", bob.Username)
		require.NoError(t, form.Validate(c, db))
	})

	t.Run("a prompt sent moments ago blocks another one, even to a different recipient", func(t *testing.T) {
		t.Parallel()

		asker2, err := factory.User(ctx, db)
		require.NoError(t, err)
		_, err = factory.PostPrompt(ctx, db, asker2.ID, bob.ID)
		require.NoError(t, err)

		c, _ := pfNewCtx(t)
		form := pfPromptForm(t, fakesender.New(), asker2, []*core.User{bob, carol}, "Tell us about your week!", carol.Username)
		require.Error(t, form.Validate(c, db))
	})
}

func TestPostPromptForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	asker, err := factory.User(ctx, db)
	require.NoError(t, err)
	recipient, err := factory.User(ctx, db)
	require.NoError(t, err)

	sender := fakesender.New()
	form := pfPromptForm(t, sender, asker, []*core.User{recipient}, "Tell us about your week!", recipient.Username)

	c, _ := pfNewCtx(t)
	require.NoError(t, form.Validate(c, db))

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	action(c, form)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "post_prompt", sent[0].EmailType)
	require.Equal(t, recipient.Email, sent[0].Mail.To[0].Address)
}
