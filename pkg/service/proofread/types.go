package proofread

import (
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/proofread"
)

// GrammarRequest is the text of a grammar check, as it is in the form.
// PostID is the post being edited, or empty for one never saved; it is
// recorded with the call when it is the actor's post.
type GrammarRequest struct {
	PostID  string
	Subject string
	Body    string
}

// StyleRequest is the text of a style review of a saved post, as it is in
// the form, and what the author wants the reviewer to know.
type StyleRequest struct {
	PostID     string
	Subject    string
	Body       string
	AuthorNote string
}

// Remaining is what is left of a user's hourly limits, with the limits.
type Remaining struct {
	Grammar      int `json:"grammar"`
	Style        int `json:"style"`
	GrammarLimit int `json:"grammar_limit"`
	StyleLimit   int `json:"style_limit"`
}

// Suggestion is a grammar suggestion as the browser gets it: it replaces the
// Occurrence-th (from zero) non-overlapping match of Before+Quote in Field
// with Replacement. There are no offsets: the browser finds the passage.
type Suggestion struct {
	Field       proofread.Field       `json:"field"`
	Kind        proofread.GrammarKind `json:"kind"`
	Quote       string                `json:"quote"`
	Before      string                `json:"before"`
	Occurrence  int                   `json:"occurrence"`
	Replacement string                `json:"replacement"`
	Explanation string                `json:"explanation"`
}

// GrammarResult is a grammar check's answer: the suggestions that apply to
// the text, in its order, and what is left of the limits.
type GrammarResult struct {
	Suggestions []Suggestion `json:"suggestions"`
	Remaining   Remaining    `json:"remaining"`
}

// Note is a style note as the browser gets it. Quote, Before and Occurrence
// locate it as for a Suggestion, in the text the review saw; a general note
// has field none and no quote. ContinuesNumber is the review the note has
// been open since when it repeats an earlier note, or 0 for a new one.
type Note struct {
	ID              string                `json:"id"`
	Field           proofread.Field       `json:"field"`
	Kind            proofread.NoteKind    `json:"kind"`
	Quote           string                `json:"quote"`
	Before          string                `json:"before"`
	Occurrence      int                   `json:"occurrence"`
	Text            string                `json:"text"`
	Status          model.StyleNoteStatus `json:"status"`
	KeepReason      string                `json:"keep_reason"`
	ContinuesNumber int                   `json:"continues_number"`
}

// Review is a style review with all its notes, open or settled.
type Review struct {
	ID         string    `json:"id"`
	Number     int       `json:"number"`
	Summary    string    `json:"summary"`
	AuthorNote string    `json:"author_note"`
	CreatedAt  time.Time `json:"created_at"`
	Notes      []Note    `json:"notes"`
}

// EarlierReview is a review before the latest, as the read-only list shows
// it.
type EarlierReview struct {
	Number    int       `json:"number"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

// StyleReview is a new style review and what is left of the limits.
type StyleReview struct {
	Review    *Review   `json:"review"`
	Remaining Remaining `json:"remaining"`
}

// StyleState is a post's style reviews: the latest with its notes (nil when
// there is none), the earlier ones newest first, and what is left of the
// limits.
type StyleState struct {
	Review    *Review         `json:"review"`
	Earlier   []EarlierReview `json:"earlier"`
	Remaining Remaining       `json:"remaining"`
}
