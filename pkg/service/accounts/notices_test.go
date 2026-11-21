package accounts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestNotices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := accounts.New(repo.New(db), nil)
	user := newUser(t, ctx, db)
	other := newUser(t, ctx, db)

	shown := testutil.Must(svc.NoticeShown(ctx, user, accounts.NoticeScratchPad))(t)
	require.True(t, shown, "shown until dismissed")

	require.NoError(t, svc.DismissNotice(ctx, user, accounts.NoticeScratchPad))
	require.NoError(t, svc.DismissNotice(ctx, user, accounts.NoticeScratchPad), "a repeat is fine")

	require.False(t, testutil.Must(svc.NoticeShown(ctx, user, accounts.NoticeScratchPad))(t))
	require.True(t, testutil.Must(svc.NoticeShown(ctx, other, accounts.NoticeScratchPad))(t), "dismissed per account")

	require.False(t, testutil.Must(svc.NoticeShown(ctx, nil, accounts.NoticeScratchPad))(t), "anonymous")
	require.False(t, testutil.Must(svc.NoticeShown(ctx, user, "nope"))(t), "unknown notice")

	require.ErrorAs(t, svc.DismissNotice(ctx, user, "nope"), new(*service.ValidationError))
	require.ErrorIs(t, svc.DismissNotice(ctx, nil, accounts.NoticeScratchPad), service.ErrNeedsLogin)
}
