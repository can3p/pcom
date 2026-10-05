package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
)

// A whitelist grant says: who allows allowsWho to connect without mediation.
// It is open until the connection is made, then it points at the connection.

// WhitelistTarget finds the user a whitelist grant is about, by username.
func (s *Store) WhitelistTarget(ctx context.Context, username string) (*model.User, error) {
	user := new(model.User)

	err := s.query().NewSelect().Model(user).
		Where("?TableAlias.username = ?", username).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return user, nil
}

// WhitelistedUsers returns the users whoID has an open grant for.
func (s *Store) WhitelistedUsers(ctx context.Context, whoID string) ([]*model.User, error) {
	var grants []*model.WhitelistedConnection

	err := s.query().NewSelect().Model(&grants).
		Relation("AllowsWho").
		Where("?TableAlias.who_id = ?", whoID).
		Where("?TableAlias.connection_id IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*model.User, len(grants))
	for i, g := range grants {
		users[i] = g.AllowsWho
	}

	return users, nil
}

// OpenGrantExists reports whether who has an open grant for allowsWho.
func (s *Store) OpenGrantExists(ctx context.Context, whoID, allowsWhoID string) (bool, error) {
	return s.query().NewSelect().Model((*model.WhitelistedConnection)(nil)).
		Where("?TableAlias.who_id = ?", whoID).
		Where("?TableAlias.allows_who_id = ?", allowsWhoID).
		Where("?TableAlias.connection_id IS NULL").
		Exists(ctx)
}

// GrantExists reports whether who has a grant for allowsWho, open or used.
func (s *Store) GrantExists(ctx context.Context, whoID, allowsWhoID string) (bool, error) {
	return s.query().NewSelect().Model((*model.WhitelistedConnection)(nil)).
		Where("?TableAlias.who_id = ?", whoID).
		Where("?TableAlias.allows_who_id = ?", allowsWhoID).
		Exists(ctx)
}

// LockOpenGrant returns the open grant of who for allowsWho and locks it
// until the transaction ends. ErrNotFound when there is none.
func (s *Store) LockOpenGrant(ctx context.Context, whoID, allowsWhoID string) (*model.WhitelistedConnection, error) {
	grant := new(model.WhitelistedConnection)

	err := s.query().NewSelect().Model(grant).
		Where("?TableAlias.who_id = ?", whoID).
		Where("?TableAlias.allows_who_id = ?", allowsWhoID).
		Where("?TableAlias.connection_id IS NULL").
		For("UPDATE").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return grant, nil
}

// UseGrant marks a grant as used by the connection.
func (s *Store) UseGrant(ctx context.Context, grant *model.WhitelistedConnection, connectionID string) error {
	grant.ConnectionID = new(connectionID)

	_, err := s.query().NewUpdate().Model(grant).WherePK().Exec(ctx)

	return err
}

// CreateGrant lets allowsWho connect to who without mediation.
func (s *Store) CreateGrant(ctx context.Context, whoID, allowsWhoID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	grant := &model.WhitelistedConnection{ID: id.String(), WhoID: whoID, AllowsWhoID: allowsWhoID}

	_, err = s.query().NewInsert().Model(grant).Exec(ctx)

	return err
}

// DeleteOpenGrant withdraws the open grant of who for allowsWho.
func (s *Store) DeleteOpenGrant(ctx context.Context, whoID, allowsWhoID string) error {
	_, err := s.query().NewDelete().Model((*model.WhitelistedConnection)(nil)).
		Where("?TableAlias.who_id = ?", whoID).
		Where("?TableAlias.allows_who_id = ?", allowsWhoID).
		Where("?TableAlias.connection_id IS NULL").
		Exec(ctx)

	return err
}
