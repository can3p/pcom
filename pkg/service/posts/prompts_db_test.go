package posts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// A prompt goes to a direct connection only; the service checks it itself,
// whatever recipient the caller passes, and stores nothing otherwise.
func TestSendPrompt_DirectConnectionsOnly(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	w := newWorld(t, ctx, db)
	s := posts.New(repo.New(db), fakesender.New(), nil)

	for _, tc := range []struct {
		name      string
		recipient *core.User
		ok        bool
	}{
		{"direct connection", w.direct, true},
		{"second degree", w.second, false},
		{"stranger", w.stranger, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := s.SendPrompt(ctx, w.author, tc.recipient, "Tell us about your week!")
			n, cerr := core.PostPrompts(core.PostPromptWhere.RecipientID.EQ(tc.recipient.ID)).Count(ctx, db)
			require.NoError(t, cerr)

			if tc.ok {
				require.NoError(t, err)
				require.EqualValues(t, 1, n)

				return
			}

			require.EqualError(t, err, "'"+tc.recipient.Username+"' is not your direct connection")
			require.Zero(t, n)
		})
	}
}
