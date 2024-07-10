package csp

import (
	"os"
	"strings"

	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
)

var cspParts = strings.Join(
	[]string{
		// all resources from https only, no inline eval
		"default-src 'self' " + os.Getenv("STATIC_CDN"),
		// forbid embedding the pages anywhere
		"frame-ancestors 'none';",
		// allow data: as a source for images
		"img-src data: w3.org/svg/2000 'self'",
	}, "; ")

func Csp(c *gin.Context) {
	c.Header("Content-Security-Policy", cspParts)
	// do not allow to load resources with mismatching mime type
	c.Header("X-Content-Type-Options", "nosniff")

	if util.InCluster() {
		// force https
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}
