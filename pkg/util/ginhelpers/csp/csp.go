package csp

import (
	"os"
	"strings"

	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	cspStyleNonceKey  = "csp_style_nonce"
	cspScriptNonceKey = "csp_script_nonce"
)

func GetStyleNonce(c *gin.Context) *string {
	v, ok := c.Get(cspStyleNonceKey)

	if !ok {
		return nil
	}

	str := v.(string)

	return &str
}

func GetScriptNonce(c *gin.Context) *string {
	v, ok := c.Get(cspScriptNonceKey)

	if !ok {
		return nil
	}

	str := v.(string)

	return &str
}

func setNonce(c *gin.Context, key, val string) {
	c.Set(key, val)
}

var cspParts = strings.Join(
	[]string{
		// all resources from https only, no inline eval
		"default-src 'self' " + os.Getenv("STATIC_CDN"),
		// forbid embedding the pages anywhere
		"frame-ancestors 'none';",
		"frame-src  www.youtube-nocookie.com",
		// allow data: as a source for images
		"img-src data: w3.org/svg/2000 'self' " + os.Getenv("STATIC_CDN") + " " + os.Getenv("USER_MEDIA_CDN") + " i.ytimg.com",
		"script-src 'self' " + os.Getenv("STATIC_CDN") + " 'nonce-SCRIPT_NONCE'",
		"style-src 'self' " + os.Getenv("STATIC_CDN") + " 'nonce-STYLE_NONCE'",
	}, "; ")

func Csp(c *gin.Context) {
	styleNonce := uuid.NewString()
	scriptNonce := uuid.NewString()

	parts := strings.Replace(cspParts, "STYLE_NONCE", styleNonce, 1)
	parts = strings.Replace(parts, "SCRIPT_NONCE", scriptNonce, 1)
	setNonce(c, cspStyleNonceKey, styleNonce)
	setNonce(c, cspScriptNonceKey, scriptNonce)

	c.Header("Content-Security-Policy", parts)
	// do not allow to load resources with mismatching mime type
	c.Header("X-Content-Type-Options", "nosniff")

	if util.InCluster() {
		// force https
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}
