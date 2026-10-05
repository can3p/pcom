package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// A mediation request asks for a connection with a second-degree user. Common
// connections (mediators) sign or dismiss it, then the target decides.

// MediationRequestBetween returns the request of who to target. ErrNotFound
// when there is none.
func (s *Store) MediationRequestBetween(ctx context.Context, whoID, targetID string) (*model.UserConnectionMediationRequest, error) {
	req, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.EQ(whoID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetID),
	).One(ctx, s.exec)

	return toModel[model.UserConnectionMediationRequest](req), notFound(err)
}

// MediationRequestExists reports whether who has a request to target.
func (s *Store) MediationRequestExists(ctx context.Context, whoID, targetID string) (bool, error) {
	return core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.EQ(whoID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetID),
	).Exists(ctx, s.exec)
}

// CreateMediationRequest stores a request of who to target with an optional note.
func (s *Store) CreateMediationRequest(ctx context.Context, whoID, targetID, note string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	req := &core.UserConnectionMediationRequest{
		ID:           id.String(),
		WhoUserID:    whoID,
		TargetUserID: targetID,
		SourceNote:   null.NewString(note, note != ""),
	}

	return req.Insert(ctx, s.exec, boil.Infer())
}

// DeleteMediationRequestBetween removes the request of who to target with its
// mediators' decisions, locking them first. ErrNotFound when there is none.
func (s *Store) DeleteMediationRequestBetween(ctx context.Context, whoID, targetID string) error {
	req, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.EQ(whoID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetID),
		qm.Load(core.UserConnectionMediationRequestRels.MediationUserConnectionMediators, qm.For("UPDATE")),
		qm.For("UPDATE"),
	).One(ctx, s.exec)
	if err != nil {
		return notFound(err)
	}

	for _, m := range req.R.MediationUserConnectionMediators {
		if _, err := m.Delete(ctx, s.exec); err != nil {
			return err
		}
	}

	_, err = req.Delete(ctx, s.exec)

	return err
}

// UndecidedRequestBetween returns request requestID when both its sides are
// among userIDs and the target has not decided yet. ErrNotFound otherwise.
func (s *Store) UndecidedRequestBetween(ctx context.Context, requestID string, userIDs []string) (*model.UserConnectionMediationRequest, error) {
	req, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.ID.EQ(requestID),
		core.UserConnectionMediationRequestWhere.WhoUserID.IN(userIDs),
		core.UserConnectionMediationRequestWhere.TargetUserID.IN(userIDs),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
	).One(ctx, s.exec)

	return toModel[model.UserConnectionMediationRequest](req), notFound(err)
}

// AddMediatorDecision records the decision of a mediator on a request.
func (s *Store) AddMediatorDecision(ctx context.Context, requestID, mediatorID string, decision model.ConnectionMediationDecision, note string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	mediator := &core.UserConnectionMediator{
		ID:           id.String(),
		MediationID:  requestID,
		UserID:       mediatorID,
		Decision:     core.ConnectionMediationDecision(decision),
		DecidedAt:    time.Now(),
		MediatorNote: null.NewString(note, note != ""),
	}

	return mediator.Insert(ctx, s.exec, boil.Infer())
}

// LockUndecidedRequest returns request requestID addressed to targetID, if the
// target has not decided yet, and locks it until the transaction ends.
// ErrNotFound otherwise.
func (s *Store) LockUndecidedRequest(ctx context.Context, requestID, targetID string) (*model.UserConnectionMediationRequest, error) {
	req, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.ID.EQ(requestID),
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetID),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.For("UPDATE"),
	).One(ctx, s.exec)

	return toModel[model.UserConnectionMediationRequest](req), notFound(err)
}

// RecordTargetDecision stores the decision of the request's target. A
// connectionID (empty for none) links the connection an approval created.
func (s *Store) RecordTargetDecision(ctx context.Context, req *model.UserConnectionMediationRequest, decision model.ConnectionRequestDecision, connectionID, note string) error {
	if connectionID != "" {
		req.ConnectionID = new(connectionID)
	}

	req.TargetDecision = new(decision)
	req.TargetDecidedAt = new(time.Now())
	req.TargetNote = nil
	if note != "" {
		req.TargetNote = new(note)
	}

	return write(req, func(c *core.UserConnectionMediationRequest) error {
		_, err := c.Update(ctx, s.exec, boil.Infer())

		return err
	})
}

// RequestsToDecide returns the undecided requests addressed to targetID that
// at least one mediator signed, with the requester and the signing mediators
// loaded.
func (s *Store) RequestsToDecide(ctx context.Context, targetID string) ([]*model.UserConnectionMediationRequest, error) {
	reqs, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(targetID),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.Load(core.UserConnectionMediationRequestRels.WhoUser),
		qm.Load(qm.Rels(
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorRels.User,
		)),
		qm.Load(core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorWhere.Decision.EQ(core.ConnectionMediationDecisionSigned),
		),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	signed := reqs[:0]
	for _, req := range reqs {
		if len(req.R.MediationUserConnectionMediators) > 0 {
			signed = append(signed, req)
		}
	}

	return toModels[model.UserConnectionMediationRequest](signed), nil
}

// RequestsToMediate returns the undecided requests between users of userIDs
// that mediatorID has not answered yet, with both sides loaded.
func (s *Store) RequestsToMediate(ctx context.Context, userIDs []string, mediatorID string) ([]*model.UserConnectionMediationRequest, error) {
	reqs, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.IN(userIDs),
		core.UserConnectionMediationRequestWhere.TargetUserID.IN(userIDs),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.Load(core.UserConnectionMediationRequestRels.WhoUser),
		qm.Load(core.UserConnectionMediationRequestRels.TargetUser),
		qm.Load(
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorWhere.UserID.EQ(mediatorID),
		),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	open := reqs[:0]
	for _, req := range reqs {
		if len(req.R.MediationUserConnectionMediators) == 0 {
			open = append(open, req)
		}
	}

	return toModels[model.UserConnectionMediationRequest](open), nil
}
