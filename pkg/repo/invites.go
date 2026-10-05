package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// OpenInvitationByID returns an invitation nobody has accepted yet, with its
// inviter loaded (invite.User), or ErrNotFound.
func (s *Store) OpenInvitationByID(ctx context.Context, id string) (*model.UserInvitation, error) {
	inv, err := core.UserInvitations(
		core.UserInvitationWhere.ID.EQ(id),
		core.UserInvitationWhere.CreatedUserID.IsNull(),
		qm.Load(core.UserInvitationRels.User),
	).One(ctx, s.exec)

	return toModel[model.UserInvitation](inv), notFound(err)
}

// PendingInvitationExists reports whether an invitation to the (normalized)
// email has been sent and not yet used to create an account.
func (s *Store) PendingInvitationExists(ctx context.Context, email string) (bool, error) {
	return core.UserInvitations(
		core.UserInvitationWhere.InvitationEmail.EQ(null.StringFrom(email)),
		core.UserInvitationWhere.CreatedUserID.IsNull(),
	).Exists(ctx, s.exec)
}

// InvitationSentTo reports whether any invitation went to the (normalized)
// email, accepted or not.
func (s *Store) InvitationSentTo(ctx context.Context, email string) (bool, error) {
	return core.UserInvitations(core.UserInvitationWhere.InvitationEmail.EQ(null.StringFrom(email))).Exists(ctx, s.exec)
}

// LockUnusedInvitation returns and locks one of the user's invitations that
// has no email yet, skipping rows locked by others, or ErrNotFound.
func (s *Store) LockUnusedInvitation(ctx context.Context, userID string) (*model.UserInvitation, error) {
	inv, err := core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(userID),
		core.UserInvitationWhere.InvitationEmail.IsNull(),
		qm.For("update skip locked"),
	).One(ctx, s.exec)

	return toModel[model.UserInvitation](inv), notFound(err)
}

// SaveInvitation writes every column of the invitation.
func (s *Store) SaveInvitation(ctx context.Context, inv *model.UserInvitation) error {
	return write(inv, func(c *core.UserInvitation) error {
		_, err := c.Update(ctx, s.exec, boil.Infer())

		return err
	})
}

// InsertInvitation inserts a new invitation.
func (s *Store) InsertInvitation(ctx context.Context, inv *model.UserInvitation) error {
	return write(inv, func(c *core.UserInvitation) error {
		return c.Insert(ctx, s.exec, boil.Infer())
	})
}

// InvitationCount counts every invitation the user owns, sent or not.
func (s *Store) InvitationCount(ctx context.Context, userID string) (int64, error) {
	return core.UserInvitations(core.UserInvitationWhere.UserID.EQ(userID)).Count(ctx, s.exec)
}

// SentInvitations returns the user's invitations that went to an email.
func (s *Store) SentInvitations(ctx context.Context, userID string) ([]*model.UserInvitation, error) {
	return all[model.UserInvitation](core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(userID),
		core.UserInvitationWhere.InvitationEmail.IsNotNull(),
	).All(ctx, s.exec))
}

// SignupRequestByID returns a waiting list entry, or ErrNotFound.
func (s *Store) SignupRequestByID(ctx context.Context, id string) (*model.UserSignupRequest, error) {
	r, err := core.UserSignupRequests(core.UserSignupRequestWhere.ID.EQ(id)).One(ctx, s.exec)

	return toModel[model.UserSignupRequest](r), notFound(err)
}

// SignupRequestEmailExists reports whether the (normalized) email waits in
// the list.
func (s *Store) SignupRequestEmailExists(ctx context.Context, email string) (bool, error) {
	return core.UserSignupRequests(core.UserSignupRequestWhere.Email.EQ(email)).Exists(ctx, s.exec)
}

// InsertSignupRequest adds an email to the waiting list; an email that is
// there already keeps its row.
func (s *Store) InsertSignupRequest(ctx context.Context, r *model.UserSignupRequest) error {
	return write(r, func(c *core.UserSignupRequest) error {
		return c.Upsert(ctx, s.exec, false, []string{core.UserSignupRequestColumns.Email}, boil.Infer(), boil.Infer())
	})
}

// SaveSignupRequest writes every column of the waiting list entry.
func (s *Store) SaveSignupRequest(ctx context.Context, r *model.UserSignupRequest) error {
	return write(r, func(c *core.UserSignupRequest) error {
		_, err := c.Update(ctx, s.exec, boil.Infer())

		return err
	})
}
