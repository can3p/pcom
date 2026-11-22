package subscriptions_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// The mail of a published post to its subscribers holds none of the text of
// the notes attached to it, one by one or by tag.
func TestRenderNewPost_HoldsNoAttachedNote(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(model.PostVisibilityPublic), factory.Published(), factory.WithBody("zqxthepostbodyitself")))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "reader@mail.test"))(t)

	oneByOne := testutil.Must(factory.Note(ctx, db, author.ID, factory.WithNoteBody("zqxattachedonebyone")))(t)
	testutil.Must(factory.Note(ctx, db, author.ID, factory.WithNoteBody("zqxbroughtbytag #walden"), factory.WithNoteTextTags("walden")))(t)
	notesSvc := notes.New(repo.New(db))
	require.NoError(t, notesSvc.Attach(ctx, author, oneByOne.ID, post.ID))
	testutil.Must(notesSvc.AttachTag(ctx, author, post.ID, "walden"))(t)
	require.Len(t, testutil.Must(notesSvc.ForPost(ctx, author, post.ID))(t).Notes, 2)

	svc, _, q := newSvc(db)
	require.NoError(t, svc.NotifyPost(ctx, nil, author, post))
	require.NoError(t, svc.FanOutPost(ctx, fakequeue.Of[subscriptions.PostPublishedArgs](q)[0]))

	sent := fakequeue.Of[subscriptions.NewPostMailArgs](q)
	require.Len(t, sent, 1)

	env := render(t, svc, sent[0])
	require.NotNil(t, env)
	// the text part carries no body, the HTML part the whole post
	require.Contains(t, env.Mail.Html, "zqxthepostbodyitself")

	for _, part := range []string{env.Mail.Subject, env.Mail.Text, env.Mail.Html} {
		require.NotContains(t, part, "zqxattachedonebyone")
		require.NotContains(t, part, "zqxbroughtbytag")
	}
}
