package subscriptions_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/stretchr/testify/require"
)

func TestSubscribable(t *testing.T) {
	t.Parallel()

	svc := subscriptions.New(nil, nil)

	for _, r := range []struct {
		name       string
		visibility model.ProfileVisibility
		enabled    bool
		want       bool
	}{
		{"public and enabled", model.ProfileVisibilityPublic, true, true},
		{"public, not enabled", model.ProfileVisibilityPublic, false, false},
		{"registered users only", model.ProfileVisibilityRegisteredUsers, true, false},
		{"connections only", model.ProfileVisibilityConnections, true, false},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			author := &model.User{ProfileVisibility: r.visibility, EmailSubscriptionsEnabled: r.enabled}
			require.Equal(t, r.want, svc.Subscribable(author))
		})
	}

	require.False(t, svc.Subscribable(nil))
}
