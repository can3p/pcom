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
// carry, NotTags the ones it must not, both read from tags || text_tags. The
// page starts right after the note (BeforeAt, BeforeID), newest capture
// first; Limit caps the rows, 0 means no cap.
type NoteFilter struct {
	UserID   string
	Tags     []string
	NotTags  []string
	SourceID string
	Query    string
	BeforeAt time.Time
	BeforeID string
	Limit    int
}

// NoteRow is a note of a list and whether it is linked to a post.
type NoteRow struct {
	Note *model.Note
	Used bool
}

type noteListRow struct {
	ID   string `bun:"id"`
	Used bool   `bun:"used"`
}

// noteTags is the union of the tags of a note, the expression the GIN index
// is on.
const noteTags = "(n.tags || n.text_tags)"

// ListNotes returns the notes the filter picks.
func (s *Store) ListNotes(ctx context.Context, f NoteFilter) ([]NoteRow, error) {
	q := s.query().NewSelect().
		TableExpr("notes AS n").
		ColumnExpr("n.id").
		ColumnExpr("EXISTS (SELECT 1 FROM note_post_links l WHERE l.note_id = n.id) AS used").
		Where("n.user_id = ?", f.UserID).
		OrderExpr("n.captured_at DESC, n.id DESC")

	if len(f.Tags) > 0 {
		q = q.Where(noteTags+" @> ?::text[]", pgdialect.Array(f.Tags))
	}

	if len(f.NotTags) > 0 {
		q = q.Where("NOT ("+noteTags+" && ?::text[])", pgdialect.Array(f.NotTags))
	}

	if f.SourceID != "" {
		q = q.Where("n.source_id = ?", f.SourceID)
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
		rows = append(rows, NoteRow{Note: byID[r.ID], Used: r.Used})
	}

	return rows, nil
}

// TagCount is a tag of a user and the notes that carry it.
type TagCount struct {
	Tag   string `bun:"tag"`
	Count int    `bun:"count"`
}

// NoteTagCounts returns the tags of the user's notes (tags || text_tags),
// most used first, then by name.
func (s *Store) NoteTagCounts(ctx context.Context, userID string) ([]TagCount, error) {
	var out []TagCount

	err := s.query().NewRaw(`
SELECT x.tag AS tag, count(*) AS count
FROM (SELECT DISTINCT n.id, unnest(`+noteTags+`) AS tag FROM notes n WHERE n.user_id = ?) x
GROUP BY x.tag
ORDER BY count(*) DESC, x.tag`, userID).Scan(ctx, &out)

	return out, err
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

// SaveNoteSource writes the title and match key of src.
func (s *Store) SaveNoteSource(ctx context.Context, src *model.NoteSource) error {
	_, err := s.query().NewUpdate().Model(src).WherePK().Column("title", "match_key").Exec(ctx)

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

// NoteSourceCounts returns all the user's sources, by title, with how many
// notes come from each.
func (s *Store) NoteSourceCounts(ctx context.Context, userID string) ([]NoteSourceCount, error) {
	var rows []struct {
		model.NoteSource `bun:",extend"`

		Notes int `bun:"notes"`
	}

	err := s.query().NewSelect().
		Model(&rows).
		ColumnExpr("?TableAlias.*").
		ColumnExpr("(SELECT count(*) FROM notes n WHERE n.source_id = ?TableAlias.id) AS notes").
		Where("?TableAlias.user_id = ?", userID).
		OrderExpr("lower(?TableAlias.title), ?TableAlias.id").Scan(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]NoteSourceCount, len(rows))
	for i := range rows {
		out[i] = NoteSourceCount{Source: &rows[i].NoteSource, Notes: rows[i].Notes}
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
