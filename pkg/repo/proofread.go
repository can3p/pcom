package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// InsertProofreadRequest records one call to the model, giving it an id
// when it has none.
func (s *Store) InsertProofreadRequest(ctx context.Context, r *model.ProofreadRequest) error {
	if err := ensureID(&r.ID); err != nil {
		return err
	}

	_, err := s.query().NewInsert().Model(r).Exec(ctx)

	return err
}

// ProofreadRequestFilter narrows the calls ProofreadRequestsSince counts. An
// empty UserID or Kind matches every user or kind.
type ProofreadRequestFilter struct {
	UserID string
	Kind   model.ProofreadKind
	Status model.ProofreadStatus
	Since  time.Time
}

// ProofreadRequestCount is how many calls matched, and when the oldest of
// them was made (zero when none did).
type ProofreadRequestCount struct {
	Count  int       `bun:"count"`
	Oldest time.Time `bun:"oldest"`
}

// ProofreadRequestsSince counts the calls with the filter's status made at
// or after its Since.
func (s *Store) ProofreadRequestsSince(ctx context.Context, f ProofreadRequestFilter) (ProofreadRequestCount, error) {
	var (
		out    ProofreadRequestCount
		oldest bun.NullTime
	)

	q := s.query().NewSelect().Model((*model.ProofreadRequest)(nil)).
		ColumnExpr("count(*)").
		ColumnExpr("min(?TableAlias.created_at)").
		Where("?TableAlias.status = ?", f.Status).
		Where("?TableAlias.created_at >= ?", f.Since)

	if f.UserID != "" {
		q = q.Where("?TableAlias.user_id = ?", f.UserID)
	}

	if f.Kind != "" {
		q = q.Where("?TableAlias.kind = ?", f.Kind)
	}

	if err := q.Scan(ctx, &out.Count, &oldest); err != nil {
		return out, err
	}

	out.Oldest = oldest.Time

	return out, nil
}

// InsertStyleReview stores a review and its notes, giving each an id and
// pointing the notes at the review.
func (s *Store) InsertStyleReview(ctx context.Context, review *model.StyleReview) error {
	if err := ensureID(&review.ID); err != nil {
		return err
	}

	if _, err := s.query().NewInsert().Model(review).Exec(ctx); err != nil {
		return err
	}

	if len(review.Notes) == 0 {
		return nil
	}

	for _, n := range review.Notes {
		if err := ensureID(&n.ID); err != nil {
			return err
		}

		n.ReviewID = review.ID
	}

	_, err := s.query().NewInsert().Model(&review.Notes).Exec(ctx)

	return err
}

// SetOpenStyleNotesStatus gives the notes among ids that are still open the
// status; notes the author has settled meanwhile keep theirs.
func (s *Store) SetOpenStyleNotesStatus(ctx context.Context, ids []string, status model.StyleNoteStatus) error {
	_, err := s.query().NewUpdate().Model((*model.StyleReviewNote)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id IN (?)", bun.List(ids)).
		Where("status = ?", model.StyleNoteStatusOpen).
		Exec(ctx)

	return err
}

// StyleReviews returns a post's reviews, newest first, each with its notes
// in their order.
func (s *Store) StyleReviews(ctx context.Context, postID string) ([]*model.StyleReview, error) {
	var reviews []*model.StyleReview

	err := s.query().NewSelect().Model(&reviews).
		Relation("Notes", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("position ASC")
		}).
		Where("?TableAlias.post_id = ?", postID).
		Order("number DESC").
		Scan(ctx)

	return reviews, err
}

// StyleReviewNote returns a note with its review loaded (note.Review).
func (s *Store) StyleReviewNote(ctx context.Context, id string) (*model.StyleReviewNote, error) {
	note := new(model.StyleReviewNote)

	err := s.query().NewSelect().Model(note).
		Relation("Review").
		Where("?TableAlias.id = ?", id).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return note, nil
}

// SetStyleNoteStatus sets a note's status and keep reason.
func (s *Store) SetStyleNoteStatus(ctx context.Context, id string, status model.StyleNoteStatus, reason string) error {
	note := &model.StyleReviewNote{ID: id, Status: status, KeepReason: reason}

	_, err := s.query().NewUpdate().Model(note).
		Column("status", "keep_reason", "updated_at").
		WherePK().
		Exec(ctx)

	return err
}

// DeleteStyleReviews removes every review of a post with its notes.
func (s *Store) DeleteStyleReviews(ctx context.Context, postID string) error {
	_, err := s.query().NewDelete().Model((*model.StyleReview)(nil)).Where("post_id = ?", postID).Exec(ctx)
	return err
}

// ensureID gives an empty id a new UUIDv7.
func ensureID(id *string) error {
	if *id != "" {
		return nil
	}

	u, err := uuid.NewV7()
	if err != nil {
		return err
	}

	*id = u.String()

	return nil
}
