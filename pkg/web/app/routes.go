package app

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var articlesRE = regexp.MustCompile("^[a-z0-9]+(_[a-z0-9]+)*$")

// mountRoutes registers every route on the groups New creates; each area's routes are in its own file.
func mountRoutes(d *Deps, router *gin.Engine, apiGroup, r, controls, actions, nonControlsForms, controlsForms *gin.RouterGroup) {
	mountMediaRoutes(d, router)
	setupApi(apiGroup, d)
	mountPublicRoutes(d, r)
	mountAuthRoutes(d, r, actions, nonControlsForms)
	mountRSSRoutes(d, r)
	mountConnectionRoutes(d, controls, controlsForms)
	mountPostRoutes(d, r, controlsForms)
	mountSettingsRoutes(d, r, controls, controlsForms)
	mountFeedRoutes(d, controlsForms)
	mountConnectionActions(d, actions)
	mountMediationActions(d, actions)
	mountPostActions(d, actions)
	mountShareActions(d, actions)
	mountPromptActions(d, actions)
	mountRSSActions(d, actions)
	mountSettingsActions(d, actions)
	mountExportActions(d, actions)
	mountMediaActions(d, actions)
}

// requireUUIDParam answers 404 when the path parameter is not a valid UUID,
// which would otherwise reach a UUID column and fail with a 500.
func requireUUIDParam(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := uuid.Parse(c.Param(name)); err != nil {
			c.AbortWithStatus(http.StatusNotFound)
		}
	}
}
