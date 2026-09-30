// Package graph answers how two users are connected. Every service that
// decides what a user may see or do builds on it. Its functions take the
// store, so they work inside the caller's transaction.
package graph

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/repo"
)

// ErrUserNotSignedIn keeps its old wording; it can reach users.
var ErrUserNotSignedIn = errors.New("One of the users is not signed in") //nolint:staticcheck

type Radius int

const (
	RadiusSameUser Radius = iota
	RadiusDirect
	RadiusSecondDegree
	RadiusUnrelated
	RadiusUnknown
)

func (r Radius) IsSameUser() bool     { return r == RadiusSameUser }
func (r Radius) IsDirect() bool       { return r == RadiusDirect }
func (r Radius) IsSecondDegree() bool { return r == RadiusSecondDegree }
func (r Radius) IsUnrelated() bool    { return r == RadiusUnrelated }

// RadiusBetween is how far toUserID is from fromUserID.
func RadiusBetween(ctx context.Context, store *repo.Store, fromUserID, toUserID string) (Radius, error) {
	if fromUserID == "" || toUserID == "" {
		return RadiusUnknown, ErrUserNotSignedIn
	}

	if fromUserID == toUserID {
		return RadiusSameUser, nil
	}

	direct, err := store.Connected(ctx, fromUserID, toUserID)
	if err != nil {
		return RadiusUnknown, err
	}

	if direct {
		return RadiusDirect, nil
	}

	second, err := store.ConnectedThroughOne(ctx, fromUserID, toUserID)
	if err != nil {
		return RadiusUnknown, err
	}

	if second {
		return RadiusSecondDegree, nil
	}

	return RadiusUnrelated, nil
}

// DirectUserIDs returns the IDs of userID's direct connections.
func DirectUserIDs(ctx context.Context, store *repo.Store, userID string) ([]string, error) {
	return store.ConnectedUserIDs(ctx, userID)
}

// DirectAndSecondDegree returns userID's direct connections, the users two
// steps away (never userID or a direct connection), and for each of those
// the direct connections they are reached through.
func DirectAndSecondDegree(ctx context.Context, store *repo.Store, userID string) (direct []string, secondDegree []string, via map[string][]string, err error) {
	direct, err = store.ConnectedUserIDs(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}

	hops, err := store.TwoHops(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}

	exclude := map[string]struct{}{userID: {}}
	for _, id := range direct {
		exclude[id] = struct{}{}
	}

	secondDegree = []string{}
	via = map[string][]string{}

	for _, hop := range hops {
		if _, ok := exclude[hop.UserID]; ok {
			continue
		}

		secondDegree = append(secondDegree, hop.UserID)
		via[hop.UserID] = append(via[hop.UserID], hop.ViaUserID)
	}

	return direct, secondDegree, via, nil
}
