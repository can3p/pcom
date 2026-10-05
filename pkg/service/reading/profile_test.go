package reading_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/stretchr/testify/require"
)

// TestCanSeeProfile tests visibility × visitor × radius combinations
func TestCanSeeProfile(t *testing.T) {
	t.Parallel()

	// Create test users
	alice := &model.User{ProfileVisibility: model.ProfileVisibilityPublic}
	bob := &model.User{ProfileVisibility: model.ProfileVisibilityRegisteredUsers}
	charlie := &model.User{ProfileVisibility: model.ProfileVisibilityConnections}

	testCases := []struct {
		name        string
		profile     *model.User
		visitor     *model.User
		connRadius  graph.Radius
		expectSee   bool
		description string
	}{
		// Public profiles: anyone can see
		{
			name:        "PublicProfile_NoVisitor",
			profile:     alice,
			visitor:     nil,
			connRadius:  graph.RadiusUnrelated,
			expectSee:   true,
			description: "Public profiles visible to anyone including anonymous",
		},
		{
			name:        "PublicProfile_RegisteredVisitor",
			profile:     alice,
			visitor:     &model.User{},
			connRadius:  graph.RadiusUnrelated,
			expectSee:   true,
			description: "Public profiles visible to registered users",
		},
		{
			name:        "PublicProfile_SameUserRadius",
			profile:     alice,
			visitor:     &model.User{},
			connRadius:  graph.RadiusSameUser,
			expectSee:   true,
			description: "Public profiles always visible regardless of radius",
		},

		// RegisteredUsers profiles: only registered users (visitor != nil)
		{
			name:        "RegisteredUsersProfile_NoVisitor",
			profile:     bob,
			visitor:     nil,
			connRadius:  graph.RadiusUnrelated,
			expectSee:   false,
			description: "RegisteredUsers profiles hidden from anonymous users",
		},
		{
			name:        "RegisteredUsersProfile_HasVisitor_Unrelated",
			profile:     bob,
			visitor:     &model.User{},
			connRadius:  graph.RadiusUnrelated,
			expectSee:   true,
			description: "RegisteredUsers profiles visible to any registered user",
		},
		{
			name:        "RegisteredUsersProfile_HasVisitor_Direct",
			profile:     bob,
			visitor:     &model.User{},
			connRadius:  graph.RadiusDirect,
			expectSee:   true,
			description: "RegisteredUsers profiles visible to connected users",
		},
		{
			name:        "RegisteredUsersProfile_HasVisitor_SecondDegree",
			profile:     bob,
			visitor:     &model.User{},
			connRadius:  graph.RadiusSecondDegree,
			expectSee:   true,
			description: "RegisteredUsers profiles visible to second-degree connections",
		},
		{
			name:        "RegisteredUsersProfile_HasVisitor_SameUser",
			profile:     bob,
			visitor:     &model.User{},
			connRadius:  graph.RadiusSameUser,
			expectSee:   true,
			description: "RegisteredUsers profiles visible to self",
		},
		{
			name:        "RegisteredUsersProfile_HasVisitor_Unknown",
			profile:     bob,
			visitor:     &model.User{},
			connRadius:  graph.RadiusUnknown,
			expectSee:   true,
			description: "RegisteredUsers profiles visible even when radius unknown",
		},

		// Connections profiles: only direct and indirect connections
		{
			name:        "ConnectionsProfile_NoVisitor",
			profile:     charlie,
			visitor:     nil,
			connRadius:  graph.RadiusUnrelated,
			expectSee:   false,
			description: "Connections profiles hidden from anonymous users",
		},
		{
			name:        "ConnectionsProfile_HasVisitor_Unrelated",
			profile:     charlie,
			visitor:     &model.User{},
			connRadius:  graph.RadiusUnrelated,
			expectSee:   false,
			description: "Connections profiles hidden from unrelated users",
		},
		{
			name:        "ConnectionsProfile_HasVisitor_Unknown",
			profile:     charlie,
			visitor:     &model.User{},
			connRadius:  graph.RadiusUnknown,
			expectSee:   false,
			description: "Connections profiles hidden when radius unknown",
		},
		{
			name:        "ConnectionsProfile_HasVisitor_Direct",
			profile:     charlie,
			visitor:     &model.User{},
			connRadius:  graph.RadiusDirect,
			expectSee:   true,
			description: "Connections profiles visible to direct connections",
		},
		{
			name:        "ConnectionsProfile_HasVisitor_SecondDegree",
			profile:     charlie,
			visitor:     &model.User{},
			connRadius:  graph.RadiusSecondDegree,
			expectSee:   true,
			description: "Connections profiles visible to second-degree connections",
		},
		{
			name:        "ConnectionsProfile_HasVisitor_SameUser",
			profile:     charlie,
			visitor:     &model.User{},
			connRadius:  graph.RadiusSameUser,
			expectSee:   true,
			description: "Users can see their own Connections profile",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reading.CanSeeProfile(tc.profile, tc.visitor, tc.connRadius)
			require.Equal(t, tc.expectSee, got, tc.description)
		})
	}
}

// TestCannotSeeProfileLite tests visibility combinations for lite check
func TestCannotSeeProfileLite(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		profileVisibility model.ProfileVisibility
		visitor           *model.User
		expectCannotSee   bool
		description       string
	}{
		// Public profiles: never blocked by lite check
		{
			name:              "PublicProfile_NoVisitor",
			profileVisibility: model.ProfileVisibilityPublic,
			visitor:           nil,
			expectCannotSee:   false,
			description:       "Public profiles are always visible, lite check returns false",
		},
		{
			name:              "PublicProfile_HasVisitor",
			profileVisibility: model.ProfileVisibilityPublic,
			visitor:           &model.User{},
			expectCannotSee:   false,
			description:       "Public profiles are always visible",
		},

		// RegisteredUsers profiles: blocked only for anonymous
		{
			name:              "RegisteredUsersProfile_NoVisitor",
			profileVisibility: model.ProfileVisibilityRegisteredUsers,
			visitor:           nil,
			expectCannotSee:   true,
			description:       "RegisteredUsers profiles blocked for anonymous users",
		},
		{
			name:              "RegisteredUsersProfile_HasVisitor",
			profileVisibility: model.ProfileVisibilityRegisteredUsers,
			visitor:           &model.User{},
			expectCannotSee:   false,
			description:       "RegisteredUsers profiles visible to registered users",
		},

		// Connections profiles: blocked only for anonymous
		{
			name:              "ConnectionsProfile_NoVisitor",
			profileVisibility: model.ProfileVisibilityConnections,
			visitor:           nil,
			expectCannotSee:   true,
			description:       "Connections profiles blocked for anonymous users",
		},
		{
			name:              "ConnectionsProfile_HasVisitor",
			profileVisibility: model.ProfileVisibilityConnections,
			visitor:           &model.User{},
			expectCannotSee:   false,
			description:       "Connections profiles visible to registered users at lite level",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			profile := &model.User{ProfileVisibility: tc.profileVisibility}
			got := reading.CannotSeeProfileLite(profile, tc.visitor)
			require.Equal(t, tc.expectCannotSee, got, tc.description)
		})
	}
}

// TestConnectionRadius tests the radius type methods
func TestConnectionRadius_IsSameUser(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		radius     graph.Radius
		expectTrue bool
	}{
		{
			name:       "SameUser",
			radius:     graph.RadiusSameUser,
			expectTrue: true,
		},
		{
			name:       "Direct",
			radius:     graph.RadiusDirect,
			expectTrue: false,
		},
		{
			name:       "SecondDegree",
			radius:     graph.RadiusSecondDegree,
			expectTrue: false,
		},
		{
			name:       "Unrelated",
			radius:     graph.RadiusUnrelated,
			expectTrue: false,
		},
		{
			name:       "Unknown",
			radius:     graph.RadiusUnknown,
			expectTrue: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.radius.IsSameUser()
			require.Equal(t, tc.expectTrue, got)
		})
	}
}

// TestConnectionRadius_IsDirect tests the IsDirect method
func TestConnectionRadius_IsDirect(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		radius     graph.Radius
		expectTrue bool
	}{
		{
			name:       "SameUser",
			radius:     graph.RadiusSameUser,
			expectTrue: false,
		},
		{
			name:       "Direct",
			radius:     graph.RadiusDirect,
			expectTrue: true,
		},
		{
			name:       "SecondDegree",
			radius:     graph.RadiusSecondDegree,
			expectTrue: false,
		},
		{
			name:       "Unrelated",
			radius:     graph.RadiusUnrelated,
			expectTrue: false,
		},
		{
			name:       "Unknown",
			radius:     graph.RadiusUnknown,
			expectTrue: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.radius.IsDirect()
			require.Equal(t, tc.expectTrue, got)
		})
	}
}

// TestConnectionRadius_IsSecondDegree tests the IsSecondDegree method
func TestConnectionRadius_IsSecondDegree(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		radius     graph.Radius
		expectTrue bool
	}{
		{
			name:       "SameUser",
			radius:     graph.RadiusSameUser,
			expectTrue: false,
		},
		{
			name:       "Direct",
			radius:     graph.RadiusDirect,
			expectTrue: false,
		},
		{
			name:       "SecondDegree",
			radius:     graph.RadiusSecondDegree,
			expectTrue: true,
		},
		{
			name:       "Unrelated",
			radius:     graph.RadiusUnrelated,
			expectTrue: false,
		},
		{
			name:       "Unknown",
			radius:     graph.RadiusUnknown,
			expectTrue: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.radius.IsSecondDegree()
			require.Equal(t, tc.expectTrue, got)
		})
	}
}

// TestConnectionRadius_IsUnrelated tests the IsUnrelated method
func TestConnectionRadius_IsUnrelated(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		radius     graph.Radius
		expectTrue bool
	}{
		{
			name:       "SameUser",
			radius:     graph.RadiusSameUser,
			expectTrue: false,
		},
		{
			name:       "Direct",
			radius:     graph.RadiusDirect,
			expectTrue: false,
		},
		{
			name:       "SecondDegree",
			radius:     graph.RadiusSecondDegree,
			expectTrue: false,
		},
		{
			name:       "Unrelated",
			radius:     graph.RadiusUnrelated,
			expectTrue: true,
		},
		{
			name:       "Unknown",
			radius:     graph.RadiusUnknown,
			expectTrue: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.radius.IsUnrelated()
			require.Equal(t, tc.expectTrue, got)
		})
	}
}
