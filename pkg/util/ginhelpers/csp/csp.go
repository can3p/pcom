package csp

import (
	"strings"

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

// Options configures the middleware.
type Options struct {
	// HSTS adds the Strict-Transport-Security header.
	HSTS bool
	// StaticCDN and MediaCDN are extra origins allowed to serve assets and
	// user media; empty means none.
	StaticCDN, MediaCDN string
}

func policy(o Options) string {
	return strings.Join(
		[]string{
			// all resources from https only, no inline eval
			"default-src 'self' " + o.StaticCDN,
			// forbid embedding the pages anywhere
			"frame-ancestors 'none';",
			"frame-src  www.youtube-nocookie.com",
			// allow data: as a source for images
			"img-src data: w3.org/svg/2000 'self' " + o.StaticCDN + " " + o.MediaCDN + " i.ytimg.com",
			"script-src 'self' " + o.StaticCDN + " 'nonce-SCRIPT_NONCE'",
			"style-src 'self' " + o.StaticCDN + " 'nonce-STYLE_NONCE'",
		}, "; ")
}

// New builds the middleware; the policy is computed once per call, not at init.
func New(o Options) gin.HandlerFunc {
	cspParts := policy(o)

	return func(c *gin.Context) {
		styleNonce := uuid.NewString()
		scriptNonce := uuid.NewString()

		parts := strings.Replace(cspParts, "STYLE_NONCE", styleNonce, 1)
		parts = strings.Replace(parts, "SCRIPT_NONCE", scriptNonce, 1)
		setNonce(c, cspStyleNonceKey, styleNonce)
		setNonce(c, cspScriptNonceKey, scriptNonce)

		c.Header("Content-Security-Policy", parts)
		// do not allow to load resources with mismatching mime type
		c.Header("X-Content-Type-Options", "nosniff")

		if o.HSTS {
			// force https
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
	}
}
