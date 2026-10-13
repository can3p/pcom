// Package proofread is what pcom asks a language model about a post and what
// it accepts back: the grammar check (suggestions to replace exact passages)
// and the style review (notes on the text as a whole, which follow up the
// post's previous review). It holds the request and answer types, the JSON
// schemas the answers must match, the system prompts, and the checks that
// tie an answer to the text it is about. It makes no calls itself: the Model
// interface is the call, implemented over the Claude API by
// pkg/proofread/claude and in memory by pkg/proofread/proofreadtest.
package proofread

import "context"

// Model is the language model the proofread service calls. Each call gets
// the model name and effort to use in its input; an error is a failed call,
// never a partial answer.
type Model interface {
	Grammar(ctx context.Context, in GrammarInput) (GrammarOutput, Usage, error)
	Style(ctx context.Context, in StyleInput) (StyleOutput, Usage, error)
}

// Effort is how hard the model works on an answer.
type Effort string

// The values of Effort.
const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// ModelChoice is the model and the effort one kind of call uses.
type ModelChoice struct {
	Name   string
	Effort Effort
}

// Usage is what a call cost: the model that answered and its token counts.
type Usage struct {
	Model        string
	InputTokens  int
	OutputTokens int
}

// Field is the part of a post an answer points at.
type Field string

// The values of Field. FieldNone is a style note about the post as a whole.
const (
	FieldSubject Field = "subject"
	FieldBody    Field = "body"
	FieldNone    Field = "none"
)

// GrammarKind is what a grammar suggestion fixes.
type GrammarKind string

// The values of GrammarKind.
const (
	GrammarKindSpelling    GrammarKind = "spelling"
	GrammarKindPunctuation GrammarKind = "punctuation"
	GrammarKindGrammar     GrammarKind = "grammar"
	GrammarKindWordChoice  GrammarKind = "word_choice"
)

// NoteKind is what a style note is about.
type NoteKind string

// The values of NoteKind.
const (
	NoteKindLogic       NoteKind = "logic"
	NoteKindRepetition  NoteKind = "repetition"
	NoteKindPacing      NoteKind = "pacing"
	NoteKindClarity     NoteKind = "clarity"
	NoteKindTitle       NoteKind = "title"
	NoteKindConsistency NoteKind = "consistency"
	NoteKindOther       NoteKind = "other"
)

// NoteStatus is where a note of an earlier review stands, as the model is
// told about it.
type NoteStatus string

// The values of NoteStatus.
const (
	NoteStatusOpen    NoteStatus = "open"
	NoteStatusDone    NoteStatus = "done"
	NoteStatusKept    NoteStatus = "kept"
	NoteStatusCarried NoteStatus = "carried"
)

// GrammarInput is a grammar check request: the text and the model to ask.
type GrammarInput struct {
	Model   ModelChoice
	Subject string
	Body    string
}

// GrammarOutput is the grammar check answer, shaped by GrammarSchema.
type GrammarOutput struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestion replaces Quote, which follows Before in Field, with
// Replacement.
type Suggestion struct {
	Field       Field       `json:"field"`
	Kind        GrammarKind `json:"kind"`
	Quote       string      `json:"quote"`
	Before      string      `json:"before"`
	Replacement string      `json:"replacement"`
	Explanation string      `json:"explanation"`
}

// StyleInput is a style review request: the text, what the author wants the
// reviewer to know, the post's previous review if there is one, and the
// model to ask.
type StyleInput struct {
	Model      ModelChoice
	Subject    string
	Body       string
	AuthorNote string
	Previous   *PreviousReview
}

// PreviousReview is the post's latest review as the next one sees it: the
// text it reviewed, its summary and its notes with what the author did about
// them.
type PreviousReview struct {
	Number     int
	Subject    string
	Body       string
	Summary    string
	AuthorNote string
	Notes      []PreviousNote
}

// PreviousNote is a note of the previous review. ID is what a new note puts
// in Continues to repeat it.
type PreviousNote struct {
	ID         string
	Field      Field
	Kind       NoteKind
	Quote      string
	Text       string
	Status     NoteStatus
	KeepReason string
}

// StyleOutput is the style review answer, shaped by StyleSchema.
type StyleOutput struct {
	Summary string `json:"summary"`
	Notes   []Note `json:"notes"`
}

// Note is one style note. Quote, which follows Before in Field, is empty for
// a general note; Continues is the ID of the previous note it repeats, or
// empty.
type Note struct {
	Field     Field    `json:"field"`
	Kind      NoteKind `json:"kind"`
	Quote     string   `json:"quote"`
	Before    string   `json:"before"`
	Text      string   `json:"text"`
	Continues string   `json:"continues"`
}
