package reading

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// The private RSS feed is read without a session: the token stands for its
// owner, and it lists exactly the posts the owner reads in their feed. A
// direct connection's posts are all there; a second-degree author's only
// when shared that far.
func TestPrivateFeed(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	reader := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	second := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, reader.ID, direct.ID)
	connect(t, db, ctx, direct.ID, second.ID)

	post := func(authorID string, vis core.PostVisibility) string {
		return testutil.Must(factory.Post(ctx, db, authorID, factory.Published(), factory.Visibility(vis)))(t).ID
	}

	want := []string{
		post(direct.ID, core.PostVisibilityDirectOnly),
		post(direct.ID, core.PostVisibilitySecondDegree),
		post(direct.ID, core.PostVisibilityPublic),
		post(second.ID, core.PostVisibilitySecondDegree),
		post(second.ID, core.PostVisibilityPublic),
	}
	post(second.ID, core.PostVisibilityDirectOnly)
	testutil.Must(factory.Post(ctx, db, direct.ID, factory.Visibility(core.PostVisibilityPublic)))(t) // a draft

	token := testutil.Must(repo.RegenerateFeedToken(ctx, db, reader.ID))(t)

	feed, err := svc.PrivateFeed(ctx, token.Token)
	require.NoError(t, err)
	require.Equal(t, reader.ID, feed.Owner.ID)
	require.ElementsMatch(t, want, lo.Map(feed.Posts, func(p *postops.Post, _ int) string { return p.ID }))

	_, err = svc.PrivateFeed(ctx, uuid.NewString())
	require.ErrorIs(t, err, service.ErrNotFound, "an unknown token")

	_, err = svc.Feed(ctx, nil, false)
	require.ErrorIs(t, err, service.ErrNeedsLogin, "the feed without a login")
}
