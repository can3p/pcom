package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// A mediation request asks for a connection with a second-degree user. Common
// connections (mediators) sign or dismiss it, then the target decides.

// MediationRequestBetween returns the request of who to target. ErrNotFound
// when there is none.
func (s *Store) MediationRequestBetween(ctx context.Context, whoID, targetID string) (*model.UserConnectionMediationRequest, error) {
	req := new(model.UserConnectionMediationRequest)

	err := s.query().NewSelect().Model(req).
		Where("?TableAlias.who_user_id = ?", whoID).
		Where("?TableAlias.target_user_id = ?", targetID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return req, nil
}

// MediationRequestExists reports whether who has a request to target.
func (s *Store) MediationRequestExists(ctx context.Context, whoID, targetID string) (bool, error) {
	return s.query().NewSelect().Model((*model.UserConnectionMediationRequest)(nil)).
		Where("?TableAlias.who_user_id = ?", whoID).
		Where("?TableAlias.target_user_id = ?", targetID).
		Exists(ctx)
}

// CreateMediationRequest stores a request of who to target with an optional note.
func (s *Store) CreateMediationRequest(ctx context.Context, whoID, targetID, note string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	req := &model.UserConnectionMediationRequest{
		ID:           id.String(),
		WhoUserID:    whoID,
		TargetUserID: targetID,
	}
	if note != "" {
		req.SourceNote = new(note)
	}

	_, err = s.query().NewInsert().Model(req).Exec(ctx)

	return err
}

// DeleteMediationRequestBetween removes the request of who to target with its
// mediators' decisions, locking them first. ErrNotFound when there is none.
func (s *Store) DeleteMediationRequestBetween(ctx context.Context, whoID, targetID string) error {
	req := new(model.UserConnectionMediationRequest)

	err := s.query().NewSelect().Model(req).
		Relation("MediationUserConnectionMediators", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.For("UPDATE")
		}).
		Where("?TableAlias.who_user_id = ?", whoID).
		Where("?TableAlias.target_user_id = ?", targetID).
		For("UPDATE").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return notFound(err)
	}

	for _, m := range req.MediationUserConnectionMediators {
		if _, err := s.query().NewDelete().Model(m).WherePK().Exec(ctx); err != nil {
			return err
		}
	}

	_, err = s.query().NewDelete().Model(req).WherePK().Exec(ctx)

	return err
}

// UndecidedRequestBetween returns request requestID when both its sides are
// among userIDs and the target has not decided yet. ErrNotFound otherwise.
func (s *Store) UndecidedRequestBetween(ctx context.Context, requestID string, userIDs []string) (*model.UserConnectionMediationRequest, error) {
	req := new(model.UserConnectionMediationRequest)

	err := s.query().NewSelect().Model(req).
		Where("?TableAlias.id = ?", requestID).
		Where("?TableAlias.who_user_id IN (?)", bun.List(userIDs)).
		Where("?TableAlias.target_user_id IN (?)", bun.List(userIDs)).
		Where("?TableAlias.target_decision IS NULL").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return req, nil
}

// AddMediatorDecision records the decision of a mediator on a request.
func (s *Store) AddMediatorDecision(ctx context.Context, requestID, mediatorID string, decision model.ConnectionMediationDecision, note string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	mediator := &model.UserConnectionMediator{
		ID:          id.String(),
		MediationID: requestID,
		UserID:      mediatorID,
		Decision:    decision,
		DecidedAt:   time.Now(),
	}
	if note != "" {
		mediator.MediatorNote = new(note)
	}

	_, err = s.query().NewInsert().Model(mediator).Exec(ctx)

	return err
}

// LockUndecidedRequest returns request requestID addressed to targetID, if the
// target has not decided yet, and locks it until the transaction ends.
// ErrNotFound otherwise.
func (s *Store) LockUndecidedRequest(ctx context.Context, requestID, targetID string) (*model.UserConnectionMediationRequest, error) {
	req := new(model.UserConnectionMediationRequest)

	err := s.query().NewSelect().Model(req).
		Where("?TableAlias.id = ?", requestID).
		Where("?TableAlias.target_user_id = ?", targetID).
		Where("?TableAlias.target_decision IS NULL").
		For("UPDATE").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return req, nil
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

	_, err := s.query().NewUpdate().Model(req).WherePK().Exec(ctx)

	return err
}

// RequestsToDecide returns the undecided requests addressed to targetID that
// at least one mediator signed, with the requester and the signing mediators
// loaded.
func (s *Store) RequestsToDecide(ctx context.Context, targetID string) ([]*model.UserConnectionMediationRequest, error) {
	var reqs []*model.UserConnectionMediationRequest

	err := s.query().NewSelect().Model(&reqs).
		Relation("WhoUser").
		Relation("MediationUserConnectionMediators", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("?TableAlias.decision = ?", model.ConnectionMediationDecisionSigned)
		}).
		Relation("MediationUserConnectionMediators.User").
		Where("?TableAlias.target_user_id = ?", targetID).
		Where("?TableAlias.target_decision IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	signed := reqs[:0]
	for _, req := range reqs {
		if len(req.MediationUserConnectionMediators) > 0 {
			signed = append(signed, req)
		}
	}

	return signed, nil
}

// RequestsToMediate returns the undecided requests between users of userIDs
// that mediatorID has not answered yet, with both sides loaded.
func (s *Store) RequestsToMediate(ctx context.Context, userIDs []string, mediatorID string) ([]*model.UserConnectionMediationRequest, error) {
	var reqs []*model.UserConnectionMediationRequest

	err := s.query().NewSelect().Model(&reqs).
		Relation("WhoUser").
		Relation("TargetUser").
		Relation("MediationUserConnectionMediators", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("?TableAlias.user_id = ?", mediatorID)
		}).
		Where("?TableAlias.who_user_id IN (?)", bun.List(userIDs)).
		Where("?TableAlias.target_user_id IN (?)", bun.List(userIDs)).
		Where("?TableAlias.target_decision IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	open := reqs[:0]
	for _, req := range reqs {
		if len(req.MediationUserConnectionMediators) == 0 {
			open = append(open, req)
		}
	}

	return open, nil
}
