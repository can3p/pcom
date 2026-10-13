package proofread

import (
	"slices"
	"strings"

	"github.com/can3p/pcom/pkg/markdown"
)

// Match is where a quote sits in its field. Start and End are byte offsets,
// for the server's own checks only: Go counts bytes and the browser counts
// UTF-16 units, so a client never gets them. It gets Needle (Before+Quote, or
// Quote alone when Before did not help) and Occurrence, the number of earlier
// non-overlapping matches of Needle, and finds the passage itself.
type Match struct {
	Start, End int
	Needle     string
	Occurrence int
}

// Locate finds quote in text. If before is given and before+quote occurs,
// that occurrence is used; otherwise the first occurrence of quote. It
// reports false for an empty quote or one the text does not contain (the
// model made it up).
func Locate(text, before, quote string) (Match, bool) {
	return locate(text, before, quote, nil)
}

// locate is Locate over the matches for which free (if not nil) holds:
// before+quote is tried first, then quote alone, each taking its first
// matching occurrence that is free.
func locate(text, before, quote string, free func(markdown.Range) bool) (Match, bool) {
	if quote == "" {
		return Match{}, false
	}

	if before != "" {
		if m, ok := locateNeedle(text, before+quote, len(before), len(quote), free); ok {
			return m, true
		}
	}

	return locateNeedle(text, quote, 0, len(quote), free)
}

// locateNeedle walks the non-overlapping matches of needle; the quote is the
// last quoteLen bytes of the needle. Occurrence is the index of the chosen
// match in that walk, which is how the browser finds the same one.
func locateNeedle(text, needle string, skip, quoteLen int, free func(markdown.Range) bool) (Match, bool) {
	n := 0

	for from := 0; from < len(text); n++ {
		i := strings.Index(text[from:], needle)
		if i < 0 {
			return Match{}, false
		}

		start := from + i + skip
		if free == nil || free(markdown.Range{Start: start, End: start + quoteLen}) {
			return Match{Start: start, End: start + quoteLen, Needle: needle, Occurrence: n}, true
		}

		from += i + len(needle)
	}

	return Match{}, false
}

// CheckedSuggestion is a grammar suggestion that passed validation, with
// where it applies.
type CheckedSuggestion struct {
	Suggestion
	Match Match
}

// CheckedNote is a style note that passed validation. A general note has an
// empty Match.Needle.
type CheckedNote struct {
	Note
	Match Match
}

// ValidateGrammar keeps the suggestions of out that apply to in: the quote
// occurs in its field, does not touch protected markdown (code, link and
// image destinations, raw HTML, block-tag lines), changes something, and does
// not overlap an earlier kept suggestion.
func ValidateGrammar(in GrammarInput, out GrammarOutput) []CheckedSuggestion {
	f := newFields(in.Subject, in.Body)

	var kept []CheckedSuggestion

	for _, s := range out.Suggestions {
		if s.Replacement == s.Quote {
			continue
		}

		m, ok := f.accept(s.Field, s.Before, s.Quote)
		if !ok {
			continue
		}

		kept = append(kept, CheckedSuggestion{Suggestion: s, Match: m})
	}

	return kept
}

// ValidateStyle keeps the notes of out that apply to in. A note with a quote
// is held to the same rules as a grammar suggestion; a note without one, or
// with the field none, is a general note (field none, no quote). Continues is
// cleared unless it names a note of the previous review.
func ValidateStyle(in StyleInput, out StyleOutput) []CheckedNote {
	f := newFields(in.Subject, in.Body)

	var previous []string
	if in.Previous != nil {
		for _, n := range in.Previous.Notes {
			previous = append(previous, n.ID)
		}
	}

	var kept []CheckedNote

	for _, n := range out.Notes {
		if !slices.Contains(previous, n.Continues) {
			n.Continues = ""
		}

		if n.Field == FieldNone || n.Quote == "" {
			n.Field, n.Quote, n.Before = FieldNone, "", ""
			kept = append(kept, CheckedNote{Note: n})

			continue
		}

		m, ok := f.accept(n.Field, n.Before, n.Quote)
		if !ok {
			continue
		}

		kept = append(kept, CheckedNote{Note: n, Match: m})
	}

	return kept
}

// fields tracks, per field, what is protected and what earlier answers have
// already claimed.
type fields struct {
	text      map[Field]string
	protected []markdown.Range
	taken     map[Field][]markdown.Range
}

func newFields(subject, body string) *fields {
	return &fields{
		text:      map[Field]string{FieldSubject: subject, FieldBody: body},
		protected: markdown.ProtectedRanges(body),
		taken:     map[Field][]markdown.Range{},
	}
}

// accept locates the quote and claims its range, or reports false if the
// field is unknown, the quote is not there, it is protected or it overlaps an
// earlier claim; a repeated quote takes its next free occurrence.
func (f *fields) accept(field Field, before, quote string) (Match, bool) {
	text, ok := f.text[field]
	if !ok {
		return Match{}, false
	}

	m, ok := locate(text, before, quote, func(r markdown.Range) bool {
		return !slices.ContainsFunc(f.taken[field], r.Overlaps)
	})
	if !ok {
		return Match{}, false
	}

	r := markdown.Range{Start: m.Start, End: m.End}

	if field == FieldBody && slices.ContainsFunc(f.protected, r.Overlaps) {
		return Match{}, false
	}

	f.taken[field] = append(f.taken[field], r)

	return m, true
}
