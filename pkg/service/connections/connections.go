// Package connections manages who is connected to whom: whitelist grants
// (people allowed to connect without mediation), connections themselves and
// mediation requests, where common connections vouch for a stranger.
package connections

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
)

// ErrNoConnectionRequest is returned when there is no mediation request to revoke.
var ErrNoConnectionRequest = service.Invalid("", "No such connection request")

var errNoSuchRequest = service.Invalid("", "No such request")

type Service struct {
	store *repo.Store
}

func New(store *repo.Store) *Service {
	return &Service{store: store}
}

// RemoveFromWhitelist withdraws the actor's open grant for userID.
func (s *Service) RemoveFromWhitelist(ctx context.Context, actor *model.User, userID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.DeleteOpenGrant(ctx, actor.ID, userID)
}

// CheckWhitelist says whether the actor may whitelist username. Every
// problem is a validation error on the "username" field.
func (s *Service) CheckWhitelist(ctx context.Context, actor *model.User, username string) error {
	_, err := s.whitelistTarget(ctx, s.store, actor, username)

	return err
}

// Whitelist lets the user called username connect to the actor without mediation.
func (s *Service) Whitelist(ctx context.Context, actor *model.User, username string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		target, err := s.whitelistTarget(ctx, tx, actor, username)
		if err != nil {
			return err
		}

		return tx.CreateGrant(ctx, actor.ID, target.ID)
	})
}

func (s *Service) whitelistTarget(ctx context.Context, store *repo.Store, actor *model.User, username string) (*model.User, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	username = strings.ToLower(strings.TrimSpace(username))

	switch username {
	case "":
		return nil, service.Invalid("username", "username is a required attribute")
	case actor.Username:
		return nil, service.Invalid("username", "Can't add yourself to the list")
	}

	target, err := store.WhitelistTarget(ctx, username)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.Invalid("username", "No such user")
	} else if err != nil {
		log.Printf("Failed to check username [%s] for existence on whitelist operation: %s", username, err.Error())
		return nil, service.Invalid("username", "Failed to lookup the username")
	}

	isConnection, err := store.Connected(ctx, actor.ID, target.ID)
	if err != nil {
		log.Printf("Failed to check username connection [%s] for existence on whitelist operation: %s", username, err.Error())
		return nil, service.Invalid("username", "Failed to lookup the your connections")
	} else if isConnection {
		return nil, service.Invalid("username", "You have this connection already")
	}

	// if the grant has a connection id, we already have one and that was an error in the previous check
	// @TODO: this should be upsert
	isWhitelisted, err := store.OpenGrantExists(ctx, actor.ID, target.ID)
	if err != nil {
		log.Printf("Failed to check username connection [%s] for existence on whitelist operation: %s", username, err.Error())
		return nil, service.Invalid("username", "Failed to lookup the your connections")
	} else if isWhitelisted {
		return nil, service.Invalid("username", "Already in the list")
	}

	return target, nil
}

// IsConnectionAllowed says whether the actor may connect to targetID: the
// target whitelisted the actor and the grant is still unused.
func (s *Service) IsConnectionAllowed(ctx context.Context, actor *model.User, targetID string) (bool, error) {
	if actor == nil {
		return false, service.ErrNeedsLogin
	}

	return s.store.OpenGrantExists(ctx, targetID, actor.ID)
}

// Connect turns a whitelist grant of targetID into a connection with the
// actor. A pending mediation request between the two is dropped.
func (s *Service) Connect(ctx context.Context, actor *model.User, targetID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		grant, err := tx.LockOpenGrant(ctx, targetID, actor.ID)
		if errors.Is(err, repo.ErrNotFound) {
			return service.Invalid("", "No connection allowed")
		} else if err != nil {
			return err
		}

		if err := tx.DeleteMediationRequestBetween(ctx, actor.ID, targetID); err != nil && !errors.Is(err, repo.ErrNotFound) {
			return err
		}

		conn, _, err := tx.CreateConnection(ctx, actor.ID, targetID)
		if err != nil {
			return err
		}

		return tx.UseGrant(ctx, grant, conn.ID)
	})
}

// Drop ends the connection between the actor and targetID, with everything
// that led to it. Without a connection it does nothing.
func (s *Service) Drop(ctx context.Context, actor *model.User, targetID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.DeleteConnectionsBetween(ctx, actor.ID, targetID)
	})
}

// MediationRequest returns the actor's mediation request to targetID, or nil.
func (s *Service) MediationRequest(ctx context.Context, actor *model.User, targetID string) (*model.UserConnectionMediationRequest, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	req, err := s.store.MediationRequestBetween(ctx, actor.ID, targetID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}

	return req, err
}

// RequestMediation asks the actor's common connections to vouch for a
// connection with targetID, a second-degree connection.
func (s *Service) RequestMediation(ctx context.Context, actor *model.User, targetID, note string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		radius, err := graph.RadiusBetween(ctx, tx, actor.ID, targetID)
		if err != nil {
			return err
		}

		if radius != graph.RadiusSecondDegree {
			return service.Invalid("", "You cannot request mediation with the user without common connections")
		}

		if exists, err := tx.MediationRequestExists(ctx, actor.ID, targetID); err != nil {
			return err
		} else if exists {
			return service.Invalid("", "Mediation has already been requested")
		}

		if exists, err := tx.GrantExists(ctx, targetID, actor.ID); err != nil {
			return err
		} else if exists {
			return service.Invalid("", "Cannot create a connection request for a whitelisted connection")
		}

		return tx.CreateMediationRequest(ctx, actor.ID, targetID, note)
	})
}

// RevokeMediation withdraws the actor's mediation request to targetID.
func (s *Service) RevokeMediation(ctx context.Context, actor *model.User, targetID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		err := tx.DeleteMediationRequestBetween(ctx, actor.ID, targetID)
		if errors.Is(err, repo.ErrNotFound) {
			return ErrNoConnectionRequest
		}

		return err
	})
}

// DecideMediation lets the actor, a direct connection of both sides of the
// request, sign or dismiss it.
func (s *Service) DecideMediation(ctx context.Context, actor *model.User, requestID string, decision model.ConnectionMediationDecision, note string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	directIDs, err := graph.DirectUserIDs(ctx, s.store, actor.ID)
	if err != nil {
		return err
	}

	req, err := s.store.UndecidedRequestBetween(ctx, requestID, directIDs)
	if errors.Is(err, repo.ErrNotFound) {
		return errNoSuchRequest
	} else if err != nil {
		return err
	}

	return s.store.AddMediatorDecision(ctx, req.ID, actor.ID, decision, note)
}

// DecideRequest lets the actor, the target of a request, approve or dismiss it.
// Approving creates the connection.
func (s *Service) DecideRequest(ctx context.Context, actor *model.User, requestID string, decision model.ConnectionRequestDecision, note string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		req, err := tx.LockUndecidedRequest(ctx, requestID, actor.ID)
		if errors.Is(err, repo.ErrNotFound) {
			return errNoSuchRequest
		} else if err != nil {
			return err
		}

		var connectionID string

		if decision == model.ConnectionRequestDecisionApproved {
			conn, _, err := tx.CreateConnection(ctx, req.WhoUserID, actor.ID)
			if err != nil {
				return err
			}

			connectionID = conn.ID
		}

		return tx.RecordTargetDecision(ctx, req, decision, connectionID, note)
	})
}
