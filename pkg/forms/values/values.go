package values

import "github.com/can3p/pcom/pkg/model/core"

type SelectValue struct {
	Label string
	Value string
}

type ValueList []SelectValue

var ProfileVisibilityValues = ValueList{
	{Label: "All registered users", Value: string(core.ProfileVisibilityRegisteredUsers)},
	{Label: "Direct and indirect connections", Value: string(core.ProfileVisibilityConnections)},
	{Label: "Public", Value: string(core.ProfileVisibilityPublic)},
}
