package reading

import (
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/graph"
)

func CanSeeProfile(profile *model.User, visitor *model.User, connRadius graph.Radius) bool {
	switch {
	case profile.ProfileVisibility == model.ProfileVisibilityPublic:
		return true
	case profile.ProfileVisibility == model.ProfileVisibilityRegisteredUsers && visitor != nil:
		return true
	case profile.ProfileVisibility == model.ProfileVisibilityConnections && connRadius != graph.RadiusUnrelated && connRadius != graph.RadiusUnknown:
		return true
	}

	return false
}

func CannotSeeProfileLite(profile *model.User, visitor *model.User) bool {
	return profile.ProfileVisibility != model.ProfileVisibilityPublic && visitor == nil
}
