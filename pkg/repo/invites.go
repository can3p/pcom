package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// OpenInvitationByID returns an invitation nobody has accepted yet, with its
// inviter loaded (invite.User), or ErrNotFound.
func (s *Store) OpenInvitationByID(ctx context.Context, id string) (*model.UserInvitation, error) {
	inv := new(model.UserInvitation)
	err := s.query().NewSelect().Model(inv).
		Relation("User").
		Where("?TableAlias.id = ?", id).
		Where("?TableAlias.created_user_id IS NULL").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return inv, nil
}

// PendingInvitationExists reports whether an invitation to the (normalized)
// email has been sent and not yet used to create an account.
func (s *Store) PendingInvitationExists(ctx context.Context, email string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserInvitation)(nil)).
		Where("invitation_email = ?", email).
		Where("created_user_id IS NULL").
		Exists(ctx)
}

// InvitationSentTo reports whether any invitation went to the (normalized)
// email, accepted or not.
func (s *Store) InvitationSentTo(ctx context.Context, email string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserInvitation)(nil)).Where("invitation_email = ?", email).Exists(ctx)
}

// LockUnusedInvitation returns and locks one of the user's invitations that
// has no email yet, skipping rows locked by others, or ErrNotFound.
func (s *Store) LockUnusedInvitation(ctx context.Context, userID string) (*model.UserInvitation, error) {
	inv := new(model.UserInvitation)
	err := s.query().NewSelect().Model(inv).
		Where("user_id = ?", userID).
		Where("invitation_email IS NULL").
		For("UPDATE SKIP LOCKED").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return inv, nil
}

// SaveInvitation writes every column of the invitation.
func (s *Store) SaveInvitation(ctx context.Context, inv *model.UserInvitation) error {
	_, err := s.query().NewUpdate().Model(inv).WherePK().Exec(ctx)

	return err
}

// InsertInvitation inserts a new invitation.
func (s *Store) InsertInvitation(ctx context.Context, inv *model.UserInvitation) error {
	_, err := s.query().NewInsert().Model(inv).Exec(ctx)

	return err
}

// InvitationCount counts every invitation the user owns, sent or not.
func (s *Store) InvitationCount(ctx context.Context, userID string) (int64, error) {
	n, err := s.query().NewSelect().Model((*model.UserInvitation)(nil)).Where("user_id = ?", userID).Count(ctx)

	return int64(n), err
}

// SentInvitations returns the user's invitations that went to an email.
func (s *Store) SentInvitations(ctx context.Context, userID string) ([]*model.UserInvitation, error) {
	var invs []*model.UserInvitation
	err := s.query().NewSelect().Model(&invs).
		Where("user_id = ?", userID).
		Where("invitation_email IS NOT NULL").
		Scan(ctx)

	return invs, err
}

// SignupRequestByID returns a waiting list entry, or ErrNotFound.
func (s *Store) SignupRequestByID(ctx context.Context, id string) (*model.UserSignupRequest, error) {
	r := new(model.UserSignupRequest)
	err := s.query().NewSelect().Model(r).Where("id = ?", id).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return r, nil
}

// SignupRequestEmailExists reports whether the (normalized) email waits in
// the list.
func (s *Store) SignupRequestEmailExists(ctx context.Context, email string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserSignupRequest)(nil)).Where("email = ?", email).Exists(ctx)
}

// InsertSignupRequest adds an email to the waiting list; an email that is
// there already keeps its row.
func (s *Store) InsertSignupRequest(ctx context.Context, r *model.UserSignupRequest) error {
	_, err := s.query().NewInsert().Model(r).On("CONFLICT (email) DO NOTHING").Exec(ctx)

	return err
}

// SaveSignupRequest writes every column of the waiting list entry.
func (s *Store) SaveSignupRequest(ctx context.Context, r *model.UserSignupRequest) error {
	_, err := s.query().NewUpdate().Model(r).WherePK().Exec(ctx)

	return err
}
