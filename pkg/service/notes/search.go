package notes

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// Search is what the notes search box holds: the tags a note must carry
// (#tag), the ones it must not (-#tag) and the free text the body is searched
// for. Tags are normalized like every tag (lowercased, at most
// DefaultTagMaxLength characters of words joined by dashes, not digits only)
// and held once each, in the order they were typed. A tag both included and
// excluded is excluded: it is only in NotTags.
type Search struct {
	Tags    []string
	NotTags []string
	Query   string
}

// ParseSearch splits a search box query into its parts. The query is read as
// words separated by spaces: "#word" is a tag, "-#word" a tag to leave out,
// and every other word, a "#" or "-#" that is not followed by a valid tag
// included, is text. The text keeps the order of its words, joined by single
// spaces.
func ParseSearch(q string) Search {
	var (
		s    Search
		text []string
	)

	for word := range strings.FieldsSeq(q) {
		if tag, ok := searchTag(word, "-#"); ok {
			s.NotTags = appendNew(s.NotTags, tag)
		} else if tag, ok := searchTag(word, "#"); ok {
			s.Tags = appendNew(s.Tags, tag)
		} else {
			text = append(text, word)
		}
	}

	for _, t := range s.NotTags {
		s.Tags = without(s.Tags, t)
	}
	s.Query = strings.Join(text, " ")

	return s
}

// searchTag returns the tag a word names after prefix, if it is one.
func searchTag(word, prefix string) (string, bool) {
	raw, ok := strings.CutPrefix(word, prefix)
	if !ok {
		return "", false
	}

	tag := strings.ToLower(raw)

	return tag, utf8.RuneCountInString(tag) <= DefaultTagMaxLength && validTag(tag)
}

func appendNew(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}

	return append(list, v)
}

// String is the canonical query of the search: its tags, then the tags left
// out, then the text, separated by spaces. ParseSearch of it gives the search
// back.
func (s Search) String() string {
	parts := make([]string, 0, len(s.Tags)+len(s.NotTags)+1)

	for _, t := range s.Tags {
		parts = append(parts, "#"+t)
	}

	for _, t := range s.NotTags {
		parts = append(parts, "-#"+t)
	}

	if q := strings.TrimSpace(s.Query); q != "" {
		parts = append(parts, q)
	}

	return strings.Join(parts, " ")
}

// IsZero reports whether the search picks nothing: no tags and no text.
func (s Search) IsZero() bool {
	return len(s.Tags) == 0 && len(s.NotTags) == 0 && strings.TrimSpace(s.Query) == ""
}

// WithTag is the search with the tag added to its tags, and taken out of the
// tags it leaves out.
func (s Search) WithTag(tag string) Search {
	s.NotTags = without(s.NotTags, tag)
	s.Tags = appendNew(slices.Clone(s.Tags), tag)

	return s
}

// WithoutTag is the search without the tag among its tags.
func (s Search) WithoutTag(tag string) Search {
	s.Tags = without(s.Tags, tag)

	return s
}

// WithoutNotTag is the search without the tag among the tags it leaves out.
func (s Search) WithoutNotTag(tag string) Search {
	s.NotTags = without(s.NotTags, tag)

	return s
}

// WithoutQuery is the search without its text.
func (s Search) WithoutQuery() Search {
	s.Query = ""

	return s
}

// without is a copy of list without v, so a Search's slices are never shared
// with the one it was made from; an empty list is nil.
func without(list []string, v string) []string {
	out := slices.DeleteFunc(slices.Clone(list), func(t string) bool { return t == v })
	if len(out) == 0 {
		return nil
	}

	return out
}
