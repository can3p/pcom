package values

import "github.com/can3p/pcom/pkg/model"

type SelectValue struct {
	Label string
	Value string
}

type ValueList []SelectValue

var ProfileVisibilityValues = ValueList{
	{Label: "All registered users", Value: string(model.ProfileVisibilityRegisteredUsers)},
	{Label: "Direct and indirect connections", Value: string(model.ProfileVisibilityConnections)},
	{Label: "Public", Value: string(model.ProfileVisibilityPublic)},
}
