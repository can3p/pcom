package postops

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

const promptTimeout = 5 * time.Minute

func CanPromptNow(ctx context.Context, exec boil.ContextExecutor, askerID string) error {
	lastPrompt, err := core.PostPrompts(
		core.PostPromptWhere.AskerID.EQ(askerID),
		qm.OrderBy("? DESC", core.PostPromptColumns.ID),
		qm.Limit(1),
	).One(ctx, exec)

	if err == sql.ErrNoRows {
		return nil
	}

	if err != nil {
		return err
	}

	return fmt.Errorf("You cannot send prompts for another %s", util.FormatDuration(time.Until(lastPrompt.CreatedAt.Add(promptTimeout))))
}
