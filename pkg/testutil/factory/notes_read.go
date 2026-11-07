package factory

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
)

// ListNotes returns userID's notes.
func ListNotes(ctx context.Context, exec repo.Executor, userID string) ([]*model.Note, error) {
	return list[model.Note](ctx, exec, "user_id = ?", userID)
}

// ListNoteImages returns the photos of the notes of userID.
func ListNoteImages(ctx context.Context, exec repo.Executor, userID string) ([]*model.NoteImage, error) {
	return list[model.NoteImage](ctx, exec, "note_id IN (SELECT id FROM notes WHERE user_id = ?)", userID)
}

// ListMediaUploads returns the uploads of userID, images and recordings.
func ListMediaUploads(ctx context.Context, exec repo.Executor, userID string) ([]*model.MediaUpload, error) {
	return list[model.MediaUpload](ctx, exec, "user_id = ?", userID)
}

// CountNoteJobs is how many River jobs of the kind are about a note of
// userID (their args' note_id), whatever their state.
func CountNoteJobs(ctx context.Context, exec repo.Executor, kind, userID string) (int, error) {
	return countJobs(ctx, exec, "kind = ? AND args->>'note_id' IN (SELECT id::text FROM notes WHERE user_id = ?)", kind, userID)
}

// CountOrphanNoteJobs is how many River jobs about a note (their args have a
// note_id) name a note that doesn't exist.
func CountOrphanNoteJobs(ctx context.Context, exec repo.Executor) (int, error) {
	return countJobs(ctx, exec, "args->>'note_id' IS NOT NULL AND NOT EXISTS (SELECT 1 FROM notes WHERE id::text = args->>'note_id')")
}

func countJobs(ctx context.Context, exec repo.Executor, where string, args ...any) (int, error) {
	var n int

	err := repo.Query(exec).NewRaw("SELECT count(*) FROM river_job WHERE "+where, args...).Scan(ctx, &n)

	return n, err
}
