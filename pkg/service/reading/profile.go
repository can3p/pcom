package reading

import (
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/graph"
)

func CanSeeProfile(profile *core.User, visitor *core.User, connRadius graph.Radius) bool {
	switch {
	case profile.ProfileVisibility == core.ProfileVisibilityPublic:
		return true
	case profile.ProfileVisibility == core.ProfileVisibilityRegisteredUsers && visitor != nil:
		return true
	case profile.ProfileVisibility == core.ProfileVisibilityConnections && connRadius != graph.RadiusUnrelated && connRadius != graph.RadiusUnknown:
		return true
	}

	return false
}

func CannotSeeProfileLite(profile *core.User, visitor *core.User) bool {
	return profile.ProfileVisibility != core.ProfileVisibilityPublic && visitor == nil
}
