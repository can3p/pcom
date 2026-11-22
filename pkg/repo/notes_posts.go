package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// UnlinkNotePost removes the link of the note to the post made the way how
// says; a link that doesn't exist is not an error.
func (s *Store) UnlinkNotePost(ctx context.Context, noteID, postID string, how model.NotePostLinkHow) error {
	_, err := s.query().NewDelete().Model((*model.NotePostLink)(nil)).
		Where("note_id = ? AND post_id = ? AND how = ?", noteID, postID, how).Exec(ctx)

	return err
}

// AddPostNoteTag attaches the tag to the post; a tag attached already keeps
// its row.
func (s *Store) AddPostNoteTag(ctx context.Context, postID, tag string) error {
	_, err := s.query().NewInsert().Model(&model.PostNoteTag{PostID: postID, Tag: tag}).
		On("CONFLICT (post_id, tag) DO NOTHING").Exec(ctx)

	return err
}

// RemovePostNoteTag detaches the tag from the post; a tag that isn't
// attached is not an error.
func (s *Store) RemovePostNoteTag(ctx context.Context, postID, tag string) error {
	_, err := s.query().NewDelete().Model((*model.PostNoteTag)(nil)).
		Where("post_id = ? AND tag = ?", postID, tag).Exec(ctx)

	return err
}

// PostNoteTags returns the tags attached to the post, by name.
func (s *Store) PostNoteTags(ctx context.Context, postID string) ([]string, error) {
	tags := []string{}

	err := s.query().NewSelect().Model((*model.PostNoteTag)(nil)).Column("tag").
		Where("post_id = ?", postID).Order("tag").Scan(ctx, &tags)

	return tags, err
}

// CountTaggedNotesOfPost returns how many notes of the user carry the tag
// (tags || text_tags) and are not excluded from the post.
func (s *Store) CountTaggedNotesOfPost(ctx context.Context, userID, postID, tag string) (int, error) {
	n, err := s.query().NewSelect().TableExpr("notes AS n").
		Where("n.user_id = ?", userID).
		Where(noteTags+" @> ?::text[]", pgdialect.Array([]string{tag})).
		Where("NOT EXISTS (SELECT 1 FROM note_post_links x WHERE x.note_id = n.id AND x.post_id = ? AND x.how = ?)",
			postID, model.NotePostLinkExcluded).
		Count(ctx)

	return int(n), err
}

// PostNoteRow is a note that belongs to a post: Attached says it was attached
// one by one, ViaTags are the post's tags it carries, by name.
type PostNoteRow struct {
	Note     *model.Note
	Attached bool
	ViaTags  []string
}

type postNoteListRow struct {
	ID       string   `bun:"id"`
	Attached bool     `bun:"attached"`
	ViaTags  []string `bun:"via_tags,array"`
}

// PostNoteRows returns the notes of the user that belong to the post: those
// attached one by one and those carrying one of its tags, less those
// excluded from it, newest capture first, with their sources and images.
func (s *Store) PostNoteRows(ctx context.Context, userID, postID string) ([]PostNoteRow, error) {
	var list []postNoteListRow

	err := s.query().NewRaw(`
		SELECT id, attached, via_tags FROM (
			SELECT n.id, n.captured_at,
				EXISTS (SELECT 1 FROM note_post_links l
					WHERE l.note_id = n.id AND l.post_id = ?0 AND l.how = ?2) AS attached,
				ARRAY(SELECT t.tag FROM post_note_tags t
					WHERE t.post_id = ?0 AND t.tag = ANY `+noteTags+` ORDER BY t.tag) AS via_tags
			FROM notes n
			WHERE n.user_id = ?1
				AND NOT EXISTS (SELECT 1 FROM note_post_links x
					WHERE x.note_id = n.id AND x.post_id = ?0 AND x.how = ?3)
		) c
		WHERE attached OR cardinality(via_tags) > 0
		ORDER BY captured_at DESC, id DESC`,
		postID, userID, model.NotePostLinkAttached, model.NotePostLinkExcluded).Scan(ctx, &list)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(list))
	for i, r := range list {
		ids[i] = r.ID
	}

	byID, err := s.notesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	rows := make([]PostNoteRow, 0, len(list))
	for _, r := range list {
		rows = append(rows, PostNoteRow{Note: byID[r.ID], Attached: r.Attached, ViaTags: r.ViaTags})
	}

	return rows, nil
}

// PostsOfNote returns the posts of the user the note belongs to, attached one
// by one or through a tag it carries and not excluded, newest first.
func (s *Store) PostsOfNote(ctx context.Context, userID, noteID string) ([]*model.Post, error) {
	posts := []*model.Post{}

	err := s.query().NewSelect().Model(&posts).
		Where("?TableAlias.user_id = ?", userID).
		Where("NOT EXISTS (SELECT 1 FROM note_post_links x WHERE x.note_id = ? AND x.post_id = ?TableAlias.id AND x.how = ?)",
			noteID, model.NotePostLinkExcluded).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("EXISTS (SELECT 1 FROM note_post_links l WHERE l.note_id = ? AND l.post_id = ?TableAlias.id AND l.how = ?)",
					noteID, model.NotePostLinkAttached).
				WhereOr("EXISTS (SELECT 1 FROM post_note_tags t JOIN notes n ON t.tag = ANY "+noteTags+
					" WHERE n.id = ? AND t.post_id = ?TableAlias.id)", noteID)
		}).
		OrderExpr("?TableAlias.created_at DESC, ?TableAlias.id DESC").
		Scan(ctx)

	return posts, err
}

// NoteLinks returns every link of the note to a post, of any kind, by post
// and kind.
func (s *Store) NoteLinks(ctx context.Context, noteID string) ([]*model.NotePostLink, error) {
	links := []*model.NotePostLink{}

	err := s.query().NewSelect().Model(&links).Where("note_id = ?", noteID).Order("post_id", "how").Scan(ctx)

	return links, err
}

// belongsToPost is the condition that a note (the column noteID names) belongs
// to a post (the expression post): attached one by one or carrying a tag
// attached to the post, and not excluded. Its placeholders, in order: when
// post is "?", the post, the excluded kind, the post, the attached kind and
// the post again; with a column for post, only the excluded and the attached
// kind.
func belongsToPost(noteID, post string) string {
	return "(NOT EXISTS (SELECT 1 FROM note_post_links x WHERE x.note_id = " + noteID + " AND x.post_id = " + post + " AND x.how = ?) " +
		"AND (EXISTS (SELECT 1 FROM note_post_links l WHERE l.note_id = " + noteID + " AND l.post_id = " + post + " AND l.how = ?) " +
		"OR EXISTS (SELECT 1 FROM post_note_tags t WHERE t.post_id = " + post + " AND t.tag = ANY " + noteTags + ")))"
}

// PostsOfNotes returns, for each of the user's notes among noteIDs, the posts
// it belongs to, newest first; a note that belongs to none has no entry.
func (s *Store) PostsOfNotes(ctx context.Context, userID string, noteIDs []string) (map[string][]*model.Post, error) {
	var pairs []struct {
		NoteID string `bun:"note_id"`
		PostID string `bun:"post_id"`
	}

	err := s.query().NewSelect().TableExpr("notes AS n").
		ColumnExpr("n.id AS note_id").ColumnExpr("p.id AS post_id").
		Join("JOIN posts p ON p.user_id = n.user_id").
		Where("n.user_id = ?", userID).
		Where("n.id IN (?)", bun.List(noteIDs)).
		Where(belongsToPost("n.id", "p.id"), model.NotePostLinkExcluded, model.NotePostLinkAttached).
		OrderExpr("p.created_at DESC, p.id DESC").
		Scan(ctx, &pairs)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(pairs))
	for _, p := range pairs {
		ids = append(ids, p.PostID)
	}

	var posts []*model.Post
	if len(ids) > 0 {
		if err := s.query().NewSelect().Model(&posts).Where("?TableAlias.id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
			return nil, err
		}
	}

	byID := make(map[string]*model.Post, len(posts))
	for _, p := range posts {
		byID[p.ID] = p
	}

	out := map[string][]*model.Post{}
	for _, p := range pairs {
		out[p.NoteID] = append(out[p.NoteID], byID[p.PostID])
	}

	return out, nil
}

// RecentPosts returns the user's newest saved posts and drafts, by when they
// were last changed, at most limit.
func (s *Store) RecentPosts(ctx context.Context, userID string, limit int) ([]*model.Post, error) {
	posts := []*model.Post{}

	err := s.query().NewSelect().Model(&posts).
		Where("?TableAlias.user_id = ?", userID).
		OrderExpr("COALESCE(?TableAlias.updated_at, ?TableAlias.created_at) DESC, ?TableAlias.id DESC").
		Limit(int64(limit)).Scan(ctx)

	return posts, err
}
