package connections

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/samber/lo"
)

// MediationRequest is a request between two of the user's connections that
// the user may sign or dismiss.
type MediationRequest struct {
	Requester *core.User
	Target    *core.User
	Request   *core.UserConnectionMediationRequest
}

// MediationResult is one mediator's decision on a request.
type MediationResult struct {
	Mediation *core.UserConnectionMediator
	Mediator  *core.User
}

// ConnectionRequest is a request addressed to the user that at least one
// mediator signed.
type ConnectionRequest struct {
	Requester  *core.User
	Request    *core.UserConnectionMediationRequest
	Mediations []*MediationResult
}

// Draft is an unpublished post of the user.
type Draft struct {
	PostID        string
	Subject       string
	LastUpdatedAt time.Time
}

// Controls is what the controls page shows.
type Controls struct {
	DirectConnections       core.UserSlice
	SecondDegreeConnections core.UserSlice
	WhitelistedConnections  core.UserSlice
	MediationRequests       []*MediationRequest
	ConnectionRequests      []*ConnectionRequest
	Drafts                  []*Draft
}

// Controls gathers the actor's connections, whitelist, pending requests and drafts.
func (s *Service) Controls(ctx context.Context, actor *core.User) (*Controls, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	directIDs, secondDegreeIDs, _, err := graph.DirectAndSecondDegree(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

	directUsers, err := s.store.ConnectionUsers(ctx, directIDs)
	if err != nil {
		return nil, err
	}

	secondDegreeUsers, err := s.store.ConnectionUsers(ctx, secondDegreeIDs)
	if err != nil {
		return nil, err
	}

	whitelisted, err := s.store.WhitelistedUsers(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	toDecide, err := s.store.RequestsToDecide(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	connectionRequests := lo.Map(toDecide, func(req *core.UserConnectionMediationRequest, _ int) *ConnectionRequest {
		return &ConnectionRequest{
			Requester: req.R.WhoUser,
			Request:   req,
			Mediations: lo.Map(req.R.MediationUserConnectionMediators, func(m *core.UserConnectionMediator, _ int) *MediationResult {
				return &MediationResult{Mediator: m.R.User, Mediation: m}
			}),
		}
	})

	toMediate, err := s.store.RequestsToMediate(ctx, directIDs, actor.ID)
	if err != nil {
		return nil, err
	}

	mediationRequests := lo.Map(toMediate, func(req *core.UserConnectionMediationRequest, _ int) *MediationRequest {
		return &MediationRequest{Requester: req.R.WhoUser, Target: req.R.TargetUser, Request: req}
	})

	rawDrafts, err := s.store.DraftPosts(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	drafts := lo.Map(rawDrafts, func(d *core.Post, _ int) *Draft {
		return &Draft{PostID: d.ID, Subject: d.Subject.String, LastUpdatedAt: d.UpdatedAt.Time}
	})

	return &Controls{
		DirectConnections:       directUsers,
		SecondDegreeConnections: secondDegreeUsers,
		WhitelistedConnections:  whitelisted,
		MediationRequests:       mediationRequests,
		ConnectionRequests:      connectionRequests,
		Drafts:                  drafts,
	}, nil
}
