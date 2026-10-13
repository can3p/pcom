package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// ProofreadKind is what a call to the model was for.
type ProofreadKind string

// The values of ProofreadKind.
const (
	ProofreadKindGrammar ProofreadKind = "grammar"
	ProofreadKindStyle   ProofreadKind = "style"
)

// ProofreadStatus is how a call to the model ended. Only ok calls count
// against a user's hourly limit.
type ProofreadStatus string

// The values of ProofreadStatus.
const (
	ProofreadStatusOK     ProofreadStatus = "ok"
	ProofreadStatusFailed ProofreadStatus = "failed"
)

// StyleNoteStatus is where a style review note stands. A note is open until
// the author marks it done or kept, or the next review makes it carried (a new
// note continues it) or done (none does).
type StyleNoteStatus string

// The values of StyleNoteStatus.
const (
	StyleNoteStatusOpen    StyleNoteStatus = "open"
	StyleNoteStatusDone    StyleNoteStatus = "done"
	StyleNoteStatusKept    StyleNoteStatus = "kept"
	StyleNoteStatusCarried StyleNoteStatus = "carried"
)

// ProofreadRequest is a row of proofread_requests: one call to the model.
type ProofreadRequest struct {
	bun.BaseModel `bun:"table:proofread_requests"`

	ID           string          `bun:"id,pk"`
	UserID       string          `bun:"user_id"`
	Kind         ProofreadKind   `bun:"kind"`
	PostID       *string         `bun:"post_id"`
	Model        string          `bun:"model"`
	InputTokens  int             `bun:"input_tokens,default:0"`
	OutputTokens int             `bun:"output_tokens,default:0"`
	Status       ProofreadStatus `bun:"status"`
	CreatedAt    time.Time       `bun:"created_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *ProofreadRequest) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}

// StyleReview is a row of style_reviews: the Number-th review of a post, with
// the subject and body it reviewed.
type StyleReview struct {
	bun.BaseModel `bun:"table:style_reviews"`

	ID         string    `bun:"id,pk"`
	PostID     string    `bun:"post_id"`
	Number     int       `bun:"number"`
	Subject    string    `bun:"subject"`
	Body       string    `bun:"body"`
	Summary    string    `bun:"summary"`
	AuthorNote string    `bun:"author_note,default:''"`
	CreatedAt  time.Time `bun:"created_at"`

	Post  *Post              `bun:"rel:belongs-to,join:post_id=id"`
	Notes []*StyleReviewNote `bun:"rel:has-many,join:id=review_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *StyleReview) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}

// StyleReviewNote is a row of style_review_notes. Field, Kind, Quote, Before
// and Text are the model's note; ContinuesID points at the note of an earlier
// review it repeats.
type StyleReviewNote struct {
	bun.BaseModel `bun:"table:style_review_notes"`

	ID          string          `bun:"id,pk"`
	ReviewID    string          `bun:"review_id"`
	Position    int             `bun:"position"`
	Field       string          `bun:"field"`
	Kind        string          `bun:"kind"`
	Quote       string          `bun:"quote"`
	Before      string          `bun:"before"`
	Text        string          `bun:"text"`
	ContinuesID *string         `bun:"continues_id"`
	Status      StyleNoteStatus `bun:"status"`
	KeepReason  string          `bun:"keep_reason,default:''"`
	UpdatedAt   time.Time       `bun:"updated_at"`

	Review *StyleReview `bun:"rel:belongs-to,join:review_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *StyleReviewNote) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, nil, &m.UpdatedAt)
	}

	return nil
}
