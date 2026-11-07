package repo

import (
	"context"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// CreateNote inserts n.
func (s *Store) CreateNote(ctx context.Context, n *model.Note) error {
	_, err := s.query().NewInsert().Model(n).Exec(ctx)

	return err
}

// NoteByID returns the note with id, whoever owns it, or ErrNotFound.
func (s *Store) NoteByID(ctx context.Context, id string) (*model.Note, error) {
	n := new(model.Note)
	if err := s.query().NewSelect().Model(n).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return n, nil
}

// LockNote returns the note with id locked for update until the transaction
// ends, or ErrNotFound.
func (s *Store) LockNote(ctx context.Context, id string) (*model.Note, error) {
	n := new(model.Note)
	if err := s.query().NewSelect().Model(n).Where("id = ?", id).For("UPDATE").Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return n, nil
}

// SaveNote writes the named columns of n (updated_at is stamped), or all of
// them when none is named.
func (s *Store) SaveNote(ctx context.Context, n *model.Note, columns ...string) error {
	q := s.query().NewUpdate().Model(n).WherePK()
	if len(columns) > 0 {
		q = q.Column(columns...)
	}

	_, err := q.Exec(ctx)

	return err
}

// DeleteNote removes the note with id and, by cascade, its images and post
// links; stt_requests keep their rows with a null note_id. A note that is
// already gone is not an error.
func (s *Store) DeleteNote(ctx context.Context, id string) error {
	_, err := s.query().NewDelete().Model((*model.Note)(nil)).Where("id = ?", id).Exec(ctx)

	return err
}

// NoteWithRelations returns the note with its source and its images (with
// their uploads, in position order) loaded, or ErrNotFound.
func (s *Store) NoteWithRelations(ctx context.Context, id string) (*model.Note, error) {
	n := new(model.Note)

	err := s.query().NewSelect().Model(n).
		Relation("Source").
		Relation("Images", func(q *bun.SelectQuery) *bun.SelectQuery { return q.Order("position") }).
		Relation("Images.Upload").
		Where("?TableAlias.id = ?", id).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return n, nil
}

// NoteFilter picks the notes of one user. Tags are the ones a note must
// carry, NotTags the ones it must not, both read from tags || text_tags.
// Unless ShowHidden, notes with a hidden tag are left out, except for the
// hidden tags named in Tags. Gap is the most time between two notes of a
// stack. The page starts right after the note (BeforeAt, BeforeID), newest
// capture first; Limit caps the rows, 0 means no cap.
type NoteFilter struct {
	UserID     string
	Tags       []string
	NotTags    []string
	ShowHidden bool
	SourceID   string
	Unused     bool
	Inbox      bool
	Query      string
	Gap        time.Duration
	BeforeAt   time.Time
	BeforeID   string
	Limit      int
}

// NoteRow is a note of a list with the stack it is in: StackID is the ID of
// the oldest note of the stack, StackSize the notes in it (whatever the
// filter), Used whether it is linked to a post.
type NoteRow struct {
	Note      *model.Note
	StackID   string
	StackSize int
	Used      bool
}

type noteListRow struct {
	ID        string `bun:"id"`
	StackID   string `bun:"stack_id"`
	StackSize int    `bun:"stack_size"`
	Used      bool   `bun:"used"`
}

// noteTags is the union of the tags of a note, the expression the GIN index
// is on.
const noteTags = "(n.tags || n.text_tags)"

// stacksSQL numbers the stacks of a user's notes: a note starts a new one
// when the previous note (lag over the capture order) is more than the gap
// older.
const stacksSQL = `
SELECT id,
  first_value(id) OVER (PARTITION BY stack_no ORDER BY captured_at, id) AS stack_id,
  count(*) OVER (PARTITION BY stack_no) AS stack_size
FROM (
  SELECT id, captured_at,
    sum(starts) OVER (ORDER BY captured_at, id) AS stack_no
  FROM (
    SELECT id, captured_at,
      CASE WHEN lag(captured_at) OVER (ORDER BY captured_at, id) IS NULL
        OR captured_at - lag(captured_at) OVER (ORDER BY captured_at, id) > make_interval(secs => ?)
      THEN 1 ELSE 0 END AS starts
    FROM notes WHERE user_id = ?
  ) m
) k`

// ListNotes returns the notes the filter picks with their stacks. The stacks
// are computed over all the user's notes, not just the picked ones.
func (s *Store) ListNotes(ctx context.Context, f NoteFilter) ([]NoteRow, error) {
	q := s.query().NewSelect().
		With("stacks", s.query().NewRaw(stacksSQL, f.Gap.Seconds(), f.UserID)).
		TableExpr("notes AS n").
		Join("JOIN stacks st ON st.id = n.id").
		ColumnExpr("n.id, st.stack_id, st.stack_size").
		ColumnExpr("EXISTS (SELECT 1 FROM note_post_links l WHERE l.note_id = n.id) AS used").
		Where("n.user_id = ?", f.UserID).
		OrderExpr("n.captured_at DESC, n.id DESC")

	if len(f.Tags) > 0 {
		q = q.Where(noteTags+" @> ?::text[]", pgdialect.Array(f.Tags))
	}

	if len(f.NotTags) > 0 {
		q = q.Where("NOT ("+noteTags+" && ?::text[])", pgdialect.Array(f.NotTags))
	}

	if !f.ShowHidden {
		q = q.Where("NOT ("+noteTags+" && ARRAY(SELECT p.tag FROM note_tag_prefs p WHERE p.user_id = ? AND p.hidden AND NOT (p.tag = ANY (?::text[]))))",
			f.UserID, pgdialect.Array(f.Tags))
	}

	if f.SourceID != "" {
		q = q.Where("n.source_id = ?", f.SourceID)
	}

	if f.Unused {
		q = q.Where("NOT EXISTS (SELECT 1 FROM note_post_links l WHERE l.note_id = n.id)")
	}

	if f.Inbox {
		q = q.Where("cardinality(" + noteTags + ") = 0")
	}

	if f.Query != "" {
		q = q.Where(`n.body ILIKE ? ESCAPE '\'`, "%"+likeEscaper.Replace(f.Query)+"%")
	}

	if !f.BeforeAt.IsZero() {
		q = q.Where("(n.captured_at, n.id) < (?, ?)", f.BeforeAt, f.BeforeID)
	}

	if f.Limit > 0 {
		q = q.Limit(int64(f.Limit))
	}

	var list []noteListRow
	if err := q.Scan(ctx, &list); err != nil {
		return nil, err
	}

	return s.noteRows(ctx, list)
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// noteRows loads the notes of a list with their sources and images, keeping
// the order of the list.
func (s *Store) noteRows(ctx context.Context, list []noteListRow) ([]NoteRow, error) {
	if len(list) == 0 {
		return nil, nil
	}

	ids := make([]string, len(list))
	for i, r := range list {
		ids[i] = r.ID
	}

	var notes []*model.Note

	err := s.query().NewSelect().Model(&notes).
		Relation("Source").
		Relation("Images", func(q *bun.SelectQuery) *bun.SelectQuery { return q.Order("position") }).
		Relation("Images.Upload").
		Where("?TableAlias.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*model.Note, len(notes))
	for _, n := range notes {
		byID[n.ID] = n
	}

	rows := make([]NoteRow, 0, len(list))
	for _, r := range list {
		rows = append(rows, NoteRow{Note: byID[r.ID], StackID: r.StackID, StackSize: r.StackSize, Used: r.Used})
	}

	return rows, nil
}

// TagCount is a tag of a user and the notes that carry it.
type TagCount struct {
	Tag    string `bun:"tag"`
	Count  int    `bun:"count"`
	Hidden bool   `bun:"hidden"`
}

// NoteTagCounts returns the tags of the user's notes (tags || text_tags),
// most used first, then by name.
func (s *Store) NoteTagCounts(ctx context.Context, userID string) ([]TagCount, error) {
	var out []TagCount

	err := s.query().NewRaw(`
SELECT x.tag AS tag, count(*) AS count, coalesce(p.hidden, false) AS hidden
FROM (SELECT DISTINCT n.id, unnest(`+noteTags+`) AS tag FROM notes n WHERE n.user_id = ?) x
LEFT JOIN note_tag_prefs p ON p.user_id = ? AND p.tag = x.tag
GROUP BY x.tag, p.hidden
ORDER BY count(*) DESC, x.tag`, userID, userID).Scan(ctx, &out)

	return out, err
}

// NeighborTagCounts returns the tags the other notes of a note's source and
// of its stack carry, with how many of those notes carry each.
func (s *Store) NeighborTagCounts(ctx context.Context, userID, noteID string, gap time.Duration) ([]TagCount, error) {
	var out []TagCount

	err := s.query().NewRaw(`
WITH stacks AS (`+stacksSQL+`)
SELECT x.tag AS tag, count(*) AS count
FROM (
  SELECT DISTINCT n.id, unnest(`+noteTags+`) AS tag
  FROM notes n
  JOIN stacks st ON st.id = n.id
  JOIN notes me ON me.id = ?
  JOIN stacks mine ON mine.id = me.id
  WHERE n.user_id = ? AND n.id <> me.id
    AND (st.stack_id = mine.stack_id OR (me.source_id IS NOT NULL AND n.source_id = me.source_id))
) x
GROUP BY x.tag
ORDER BY count(*) DESC, x.tag`, gap.Seconds(), userID, noteID, userID).Scan(ctx, &out)

	return out, err
}

// SetTagHidden stores whether the user hides the tag.
func (s *Store) SetTagHidden(ctx context.Context, userID, tag string, hidden bool) error {
	_, err := s.query().NewInsert().Model(&model.NoteTagPref{UserID: userID, Tag: tag, Hidden: hidden}).
		On("CONFLICT (user_id, tag) DO UPDATE").Set("hidden = EXCLUDED.hidden").Exec(ctx)

	return err
}

// NoteTagPref returns the user's preference for the tag, or nil.
func (s *Store) NoteTagPref(ctx context.Context, userID, tag string) (*model.NoteTagPref, error) {
	p := new(model.NoteTagPref)

	return orNil(p, s.query().NewSelect().Model(p).Where("user_id = ?", userID).Where("tag = ?", tag).Scan(ctx))
}

// DeleteNoteTagPref removes the user's preference for the tag.
func (s *Store) DeleteNoteTagPref(ctx context.Context, userID, tag string) error {
	_, err := s.query().NewDelete().Model((*model.NoteTagPref)(nil)).
		Where("user_id = ?", userID).Where("tag = ?", tag).Exec(ctx)

	return err
}

// LockNotesWithTags returns the user's notes that carry any of the tags (in
// either column), locked for update until the transaction ends.
func (s *Store) LockNotesWithTags(ctx context.Context, userID string, tags []string) ([]*model.Note, error) {
	var notes []*model.Note

	err := s.query().NewSelect().Model(&notes).
		Where("user_id = ?", userID).
		Where("(tags || text_tags) && ?::text[]", pgdialect.Array(tags)).
		OrderExpr("id").For("UPDATE").Scan(ctx)

	return notes, err
}

// NoteSourceByID returns the source with id, whoever owns it, or ErrNotFound.
func (s *Store) NoteSourceByID(ctx context.Context, id string) (*model.NoteSource, error) {
	src := new(model.NoteSource)
	if err := s.query().NewSelect().Model(src).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return src, nil
}

// NoteSourceByMatchKey returns the user's source with the match key, or nil.
func (s *Store) NoteSourceByMatchKey(ctx context.Context, userID, matchKey string) (*model.NoteSource, error) {
	src := new(model.NoteSource)

	return orNil(src, s.query().NewSelect().Model(src).
		Where("user_id = ?", userID).Where("match_key = ?", matchKey).Scan(ctx))
}

// CreateNoteSource inserts src, or returns the user's source with the same
// match key when one exists already.
func (s *Store) CreateNoteSource(ctx context.Context, src *model.NoteSource) (*model.NoteSource, error) {
	res, err := s.query().NewInsert().Model(src).On("CONFLICT (user_id, match_key) DO NOTHING").Exec(ctx)
	if err != nil {
		return nil, err
	}

	if n, _ := res.RowsAffected(); n == 1 {
		return src, nil
	}

	return s.NoteSourceByMatchKey(ctx, src.UserID, src.MatchKey)
}

// SaveNoteSource writes the kind, title, author, URL and match key of src.
func (s *Store) SaveNoteSource(ctx context.Context, src *model.NoteSource) error {
	_, err := s.query().NewUpdate().Model(src).WherePK().
		Column("kind", "title", "author", "url", "match_key").Exec(ctx)

	return err
}

// DeleteNoteSource removes a source; its notes keep existing without one.
func (s *Store) DeleteNoteSource(ctx context.Context, id string) error {
	_, err := s.query().NewDelete().Model((*model.NoteSource)(nil)).Where("id = ?", id).Exec(ctx)

	return err
}

// SetNotesSource points the notes at a source, or at none when sourceID is
// nil.
func (s *Store) SetNotesSource(ctx context.Context, noteIDs []string, sourceID *string) error {
	_, err := s.query().NewUpdate().Model((*model.Note)(nil)).
		Set("source_id = ?", sourceID).Set("updated_at = now()").
		Where("id IN (?)", bun.List(noteIDs)).Exec(ctx)

	return err
}

// MoveNotesSource moves the notes of the sources to another one.
func (s *Store) MoveNotesSource(ctx context.Context, fromIDs []string, toID string) error {
	_, err := s.query().NewUpdate().Model((*model.Note)(nil)).
		Set("source_id = ?", toID).Set("updated_at = now()").
		Where("source_id IN (?)", bun.List(fromIDs)).Exec(ctx)

	return err
}

// NoteSourceCount is a source and how many notes come from it.
type NoteSourceCount struct {
	Source *model.NoteSource
	Notes  int
}

// NoteSourcesWithNotes returns the user's sources that have notes, by title.
func (s *Store) NoteSourcesWithNotes(ctx context.Context, userID string) ([]NoteSourceCount, error) {
	var rows []struct {
		ID       string  `bun:"id"`
		UserID   string  `bun:"user_id"`
		Kind     string  `bun:"kind"`
		Title    string  `bun:"title"`
		Author   *string `bun:"author"`
		URL      *string `bun:"url"`
		MatchKey string  `bun:"match_key"`
		Notes    int     `bun:"notes"`
	}

	err := s.query().NewSelect().
		TableExpr("note_sources AS s").
		ColumnExpr("s.*").
		ColumnExpr("(SELECT count(*) FROM notes n WHERE n.source_id = s.id) AS notes").
		Where("s.user_id = ?", userID).
		Where("EXISTS (SELECT 1 FROM notes n WHERE n.source_id = s.id)").
		OrderExpr("lower(s.title), s.id").Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	out := make([]NoteSourceCount, len(rows))
	for i, r := range rows {
		out[i] = NoteSourceCount{
			Source: &model.NoteSource{
				ID: r.ID, UserID: r.UserID, Kind: model.NoteSourceKind(r.Kind), Title: r.Title,
				Author: r.Author, URL: r.URL, MatchKey: r.MatchKey,
			},
			Notes: r.Notes,
		}
	}

	return out, nil
}

// LinkNotePost records that the note was used in the post; a pair that is
// already linked keeps its row.
func (s *Store) LinkNotePost(ctx context.Context, noteID, postID string, how model.NotePostLinkHow) error {
	_, err := s.query().NewInsert().Model(&model.NotePostLink{NoteID: noteID, PostID: postID, How: how}).
		On("CONFLICT (note_id, post_id) DO NOTHING").Exec(ctx)

	return err
}

// SetNotesFocus stores the user's focus tag and when it was last used; a nil
// tag clears it.
func (s *Store) SetNotesFocus(ctx context.Context, userID string, tag *string, at *time.Time) error {
	_, err := s.query().NewUpdate().Model((*model.User)(nil)).
		Set("notes_focus_tag = ?", tag).Set("notes_focus_at = ?", at).
		Where("id = ?", userID).Exec(ctx)

	return err
}

// NoteUsesAudio reports whether a note has the upload as its recording.
func (s *Store) NoteUsesAudio(ctx context.Context, uploadID string) (bool, error) {
	return s.query().NewSelect().Model((*model.Note)(nil)).Where("audio_upload_id = ?", uploadID).Exists(ctx)
}

// LatestSTTSeconds is the seconds of the newest ok transcription of the
// note, or zero when it has none.
func (s *Store) LatestSTTSeconds(ctx context.Context, noteID string) (int, error) {
	var seconds []int

	err := s.query().NewSelect().Model((*model.STTRequest)(nil)).
		Column("seconds").
		Where("note_id = ?", noteID).
		Where("status = ?", model.STTStatusOK).
		OrderExpr("created_at desc").
		Limit(1).
		Scan(ctx, &seconds)
	if err != nil || len(seconds) == 0 {
		return 0, err
	}

	return seconds[0], nil
}
