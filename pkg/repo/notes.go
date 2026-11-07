package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
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
