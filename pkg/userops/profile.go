package userops

import "github.com/can3p/pcom/pkg/model/core"

func CanSeeProfile(profile *core.User, visitor *core.User, connRadius ConnectionRadius) bool {
	switch {
	case profile.ProfileVisibility == core.ProfileVisibilityPublic:
		return true
	case profile.ProfileVisibility == core.ProfileVisibilityRegisteredUsers && visitor != nil:
		return true
	case profile.ProfileVisibility == core.ProfileVisibilityConnections && !(connRadius == ConnectionRadiusUnrelated || connRadius == ConnectionRadiusUnknown):
		return true
	}

	return false
}

func CannotSeeProfileLite(profile *core.User, visitor *core.User) bool {
	return profile.ProfileVisibility != core.ProfileVisibilityPublic && visitor == nil
}
