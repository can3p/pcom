package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// OpenPromptsFor returns the prompts sent to recipientID that they haven't
// dismissed, newest first, with the asker and the answer post loaded.
func (s *Store) OpenPromptsFor(ctx context.Context, recipientID string) (core.PostPromptSlice, error) {
	return core.PostPrompts(
		core.PostPromptWhere.RecipientID.EQ(recipientID),
		core.PostPromptWhere.DismissedAt.IsNull(),
		qm.Load(core.PostPromptRels.Asker),
		qm.Load(core.PostPromptRels.Post),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostPromptColumns.CreatedAt)),
	).All(ctx, s.exec)
}
