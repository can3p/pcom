package app

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// manifestPath is where the web app manifest is served. It is the app's own
// origin and a fixed URL, not a hashed static asset: the manifest's
// start_url, scope and share target must be on the origin of the pages, and
// an installed app looks for its manifest at the URL it was installed from.
const manifestPath = "/site.webmanifest"

// siteManifest is the web app manifest; its icons name frontend assets
// (static_asset keys), which the served manifest replaces with their URLs.
//
//go:embed site.webmanifest
var siteManifest []byte

// mountManifestRoute serves the web app manifest.
func mountManifestRoute(d *Deps, router *gin.Engine) {
	body, err := renderManifest(siteManifest, d.Config.StaticAsset)
	if err != nil {
		panic(err)
	}

	router.GET(manifestPath, func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/manifest+json", body)
	})
}

// renderManifest gives every icon of the manifest src its asset URL. The
// URL may be on the static CDN: icons, unlike the app's pages, may come
// from any origin.
func renderManifest(src []byte, staticAsset StaticAssetFunc) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(src, &m); err != nil {
		return nil, fmt.Errorf("site.webmanifest: %w", err)
	}

	icons, _ := m["icons"].([]any)
	for _, i := range icons {
		icon, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("site.webmanifest: an icon is not an object")
		}

		name, _ := icon["src"].(string)
		icon["src"] = staticAsset(name)
	}

	return json.Marshal(m)
}
