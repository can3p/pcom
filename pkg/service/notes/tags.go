package notes

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
)

// TagCount is a tag of the actor and how many notes carry it.
type TagCount = repo.TagCount

// maxSuggestions is how many tags SuggestTags offers.
const maxSuggestions = 3

const tagWord = `[\p{L}\p{N}_]+(?:-[\p{L}\p{N}_]+)*`

var (
	// tagRe is what a tag may be: words joined by dashes.
	tagRe = regexp.MustCompile(`^` + tagWord + `$`)
	// hashtagRe finds a #tag in a body. The tag starts the text or follows
	// something that can't belong to a word, a URL or another tag, so
	// "a/b#c" and "&#39;" aren't tags. Group 1 is what comes before the #,
	// group 2 the tag.
	hashtagRe = regexp.MustCompile(`(^|[^\p{L}\p{N}_&#/])#(` + tagWord + `)`)
)

// normalizeTag lowercases and trims a tag and drops a leading #. It reports
// a field error when the tag can't be one.
func (s *Service) normalizeTag(field, raw string) (string, error) {
	tag := strings.ToLower(strings.TrimLeft(strings.TrimSpace(raw), "#"))

	switch {
	case tag == "":
		return "", service.Invalid(field, "A tag can't be empty.")
	case utf8.RuneCountInString(tag) > s.limits.TagMaxLength:
		return "", service.Invalid(field, fmt.Sprintf("A tag can be at most %d characters long.", s.limits.TagMaxLength))
	case !validTag(tag):
		return "", service.Invalid(field, "A tag can have letters, digits, dashes and underscores, and not digits only.")
	}

	return tag, nil
}

// validTag reports whether tag is made of words joined by dashes and is not
// all digits, so "#2024" and "#1" in a text are numbers, not tags.
func validTag(tag string) bool {
	return tagRe.MatchString(tag) && strings.ContainsFunc(tag, func(r rune) bool { return !unicode.IsDigit(r) })
}

// cleanTags normalizes tags set by an action and drops the duplicates.
func (s *Service) cleanTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))

	for _, raw := range tags {
		if strings.TrimSpace(raw) == "" {
			continue
		}

		tag, err := s.normalizeTag("tags", raw)
		if err != nil {
			return nil, err
		}

		if !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}

	return out, nil
}

// textTags returns the tags written as #words in body, in the order of their
// first appearance. Words that can't be tags are not tags.
func textTags(body string) []string {
	out := []string{}

	for _, m := range hashtagRe.FindAllStringSubmatch(body, -1) {
		tag := strings.ToLower(m[2])
		if validTag(tag) && !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}

	return out
}

// rewriteHashtags replaces the #words of body that name a key of renames
// with #<value>.
func rewriteHashtags(body string, renames map[string]string) string {
	var b strings.Builder

	last := 0

	for _, m := range hashtagRe.FindAllStringSubmatchIndex(body, -1) {
		to, ok := renames[strings.ToLower(body[m[4]:m[5]])]
		if !ok {
			continue
		}

		b.WriteString(body[last:m[4]])
		b.WriteString(to)

		last = m[5]
	}

	b.WriteString(body[last:])

	return b.String()
}

// tagsOf validates what a note is saved with and returns the tags set by
// actions and the ones the body carries: the body and each tag may not be too
// long, and the note may not have more tags than the limit.
func (s *Service) tagsOf(body string, tags []string) (set, text []string, err error) {
	if utf8.RuneCountInString(body) > s.limits.BodyMaxLength {
		return nil, nil, service.Invalid("body", fmt.Sprintf("A note can be at most %d characters long.", s.limits.BodyMaxLength))
	}

	set, err = s.cleanTags(tags)
	if err != nil {
		return nil, nil, err
	}

	text = textTags(body)
	for _, tag := range text {
		if utf8.RuneCountInString(tag) > s.limits.TagMaxLength {
			return nil, nil, service.Invalid("body", fmt.Sprintf("A tag can be at most %d characters long.", s.limits.TagMaxLength))
		}
	}

	if err := s.checkTagCount(set, text); err != nil {
		return nil, nil, err
	}

	return set, text, nil
}

// checkTagCount rejects a note that would carry more distinct tags than the
// limit.
func (s *Service) checkTagCount(set, text []string) error {
	if n := len(union(set, text)); n > s.limits.TagsMax {
		return service.Invalid("tags", fmt.Sprintf("A note can have at most %d tags.", s.limits.TagsMax))
	}

	return nil
}

// union is the distinct tags of both lists.
func union(a, b []string) []string {
	out := slices.Clone(a)

	for _, t := range b {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}

	return out
}

// Tags returns the actor's tags with their counts, most used first.
func (s *Service) Tags(ctx context.Context, actor *model.User) ([]TagCount, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.NoteTagCounts(ctx, actor.ID)
}

// SuggestTags returns up to three of the actor's tags for a note: the ones
// whose words appear in the note's text come first, then the ones the other
// notes of its source carry, most carried first. Tags the note has
// are not suggested.
func (s *Service) SuggestTags(ctx context.Context, actor *model.User, noteID string) ([]string, error) {
	note, err := s.ownNote(ctx, s.store, actor, noteID, read)
	if err != nil {
		return nil, err
	}

	counts, err := s.store.NoteTagCounts(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	fromSource, err := s.store.SourceTagCounts(ctx, actor.ID, noteID)
	if err != nil {
		return nil, err
	}

	have := union(note.Tags, note.TextTags)

	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(note.Body), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		words[w] = true
	}

	var out []string

	add := func(tag string) {
		if len(out) < maxSuggestions && !slices.Contains(have, tag) && !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}

	for _, c := range counts {
		if inText(c.Tag, words) {
			add(c.Tag)
		}
	}

	for _, c := range fromSource {
		add(c.Tag)
	}

	return out, nil
}

// inText reports whether every word of the tag is among words.
func inText(tag string, words map[string]bool) bool {
	for _, w := range strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' }) {
		if !words[w] {
			return false
		}
	}

	return true
}

// TagMany adds tags to the notes, all or none: a note that would have too
// many tags, or one that isn't the actor's, fails the whole call.
func (s *Service) TagMany(ctx context.Context, actor *model.User, noteIDs, tags []string) error {
	tags, err := s.cleanTags(tags)
	if err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		notes, err := s.ownNotes(ctx, tx, actor, noteIDs, true)
		if err != nil {
			return err
		}

		for _, n := range notes {
			n.Tags = union(n.Tags, tags)
			if err := s.checkTagCount(n.Tags, n.TextTags); err != nil {
				return err
			}

			if err := tx.SaveNote(ctx, n, "tags", "updated_at"); err != nil {
				return err
			}
		}

		return nil
	})
}

// RenameTag renames a tag in the actor's notes and the #words of their
// bodies. Renaming to a tag that exists merges them.
func (s *Service) RenameTag(ctx context.Context, actor *model.User, from, to string) error {
	return s.MergeTags(ctx, actor, []string{from}, to)
}

// MergeTags rewrites the tags into one: in the notes' tags and text_tags
// and in the #words of the bodies.
func (s *Service) MergeTags(ctx context.Context, actor *model.User, from []string, into string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	into, err := s.normalizeTag("tag", into)
	if err != nil {
		return err
	}

	renames := map[string]string{}

	for _, raw := range from {
		tag, err := s.normalizeTag("tag", raw)
		if err != nil {
			return err
		}

		if tag != into {
			renames[tag] = into
		}
	}

	if len(renames) == 0 {
		return nil
	}

	olds := make([]string, 0, len(renames))
	for tag := range renames {
		olds = append(olds, tag)
	}

	slices.Sort(olds)

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		notes, err := tx.LockNotesWithTags(ctx, actor.ID, olds)
		if err != nil {
			return err
		}

		for _, n := range notes {
			n.Body = rewriteHashtags(n.Body, renames)
			n.Tags = renameIn(n.Tags, renames)
			n.TextTags = textTags(n.Body)

			if err := tx.SaveNote(ctx, n, "body", "tags", "text_tags", "updated_at"); err != nil {
				return err
			}
		}

		return nil
	})
}

// renameIn replaces the tags that are keys of renames, without duplicates.
func renameIn(tags []string, renames map[string]string) []string {
	out := make([]string, 0, len(tags))

	for _, t := range tags {
		if to, ok := renames[t]; ok {
			t = to
		}

		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}

	return out
}
