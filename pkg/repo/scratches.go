package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// The pad of a new post is keyed by its user and prompt. Every write is one
// statement that names the revision it expects, so of two tabs writing at
// once the database decides which one wins, and the loser's write matches no
// row.

// scratchKeyExpr is the expression of post_scratches_user_prompt_idx, the
// conflict target of TakeOverScratch.
const scratchKeyExpr = "(user_id, (coalesce(prompt_id, '00000000-0000-0000-0000-000000000000'::uuid)))"

// scratchColumns are the columns a write of the pad's text sets.
var scratchColumns = []string{"subject", "url", "body", "visibility", "notify_subscribers", "note_ids", "updated_at"}

// whereScratch narrows a query to the pad of the user and prompt; an empty
// promptID is the plain pad.
func whereScratch[Q interface {
	Where(string, ...any) Q
}](q Q, userID, promptID string) Q {
	q = q.Where("?TableAlias.user_id = ?", userID)

	if promptID == "" {
		return q.Where("?TableAlias.prompt_id IS NULL")
	}

	return q.Where("?TableAlias.prompt_id = ?", promptID)
}

// ScratchOf returns the user's pad for the prompt (the plain pad for an empty
// promptID), or ErrNotFound if it was never written.
func (s *Store) ScratchOf(ctx context.Context, userID, promptID string) (*model.PostScratch, error) {
	pad := new(model.PostScratch)
	err := whereScratch(s.query().NewSelect().Model(pad), userID, promptID).Limit(1).Scan(ctx)

	return pad, notFound(err)
}

// InsertScratch inserts the pad at revision 1 unless the user already has one
// for its prompt, and reports whether it did.
func (s *Store) InsertScratch(ctx context.Context, pad *model.PostScratch) (bool, error) {
	pad.Revision = 1

	res, err := s.query().NewInsert().Model(pad).On("CONFLICT DO NOTHING").Returning("NULL").Exec(ctx)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// UpdateScratch writes the pad's text over the user's pad for its prompt if
// that is still at revision, bumps the revision and clears PromotedPostID. It
// reports whether the revision matched; pad then holds the row as written.
func (s *Store) UpdateScratch(ctx context.Context, pad *model.PostScratch, revision int) (bool, error) {
	q := s.query().NewUpdate().Model(pad).
		Column(scratchColumns...).
		Set("revision = ?TableAlias.revision + 1").
		Set("promoted_post_id = NULL").
		Where("?TableAlias.revision = ?", revision)

	err := whereScratch(q, pad.UserID, strOrEmpty(pad.PromptID)).Returning("*").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	return err == nil, err
}

// EmptyScratch empties the user's pad for the prompt if it is still at
// revision, bumps the revision and records promotedPostID, the post the text
// became (nil for a discard). It reports whether the revision matched.
func (s *Store) EmptyScratch(ctx context.Context, userID, promptID string, revision int, promotedPostID *string, at time.Time) (bool, error) {
	q := s.query().NewUpdate().Model((*model.PostScratch)(nil)).
		Set("subject = NULL").
		Set("url = NULL").
		Set("body = ''").
		Set("notify_subscribers = NULL").
		Set("note_ids = '{}'").
		Set("revision = ?TableAlias.revision + 1").
		Set("promoted_post_id = ?", promotedPostID).
		Set("updated_at = ?", at).
		Where("?TableAlias.revision = ?", revision)

	res, err := whereScratch(q, userID, promptID).Exec(ctx)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// TakeOverScratch writes the pad's text over the user's pad for its prompt
// whatever its revision, inserting it at revision 1 if there is none, bumps
// the revision and clears PromotedPostID. pad then holds the row as written.
func (s *Store) TakeOverScratch(ctx context.Context, pad *model.PostScratch) error {
	pad.Revision = 1

	q := s.query().NewInsert().Model(pad).
		On("CONFLICT " + scratchKeyExpr + " DO UPDATE").
		Set("revision = ?TableAlias.revision + 1").
		Set("promoted_post_id = NULL")

	for _, col := range scratchColumns {
		q = q.Set(col + " = EXCLUDED." + col)
	}

	_, err := q.Returning("*").Exec(ctx)

	return err
}

// strOrEmpty is the string a pointer holds, empty for nil.
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}
