package app

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"time"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/types"
	"github.com/can3p/pcom/pkg/util"
	"github.com/can3p/pcom/pkg/util/date"
)

var staticRoute = "/static"

func funcmap(staticAsset StaticAssetFunc) template.FuncMap {
	markdown := func(view types.HTMLView) func(s string, add ...string) template.HTML {
		return func(s string, add ...string) template.HTML {
			return markdown.ToEnrichedTemplate(s, view, links.MediaReplacer, func(in string, add2 ...string) string {
				// ugly hack to handle cut links
				if in == "single_post_special" {
					args := []string{}
					args = append(args, add...)
					args = append(args, add2...)

					return links.Link("post", args...)
				}

				return links.Link(in, add2...)
			})
		}
	}

	return template.FuncMap{
		"static_asset": staticAsset,

		"link": links.Link,

		"abslink": links.AbsLink,

		"renderHumanTime": func(t time.Time, user *core.User) template.HTML {
			return date.RenderTimeHTML(t, user, time.Now())
		},

		"toMap": func(args ...any) map[string]any {
			if len(args)%2 != 0 {
				panic("toMap got uneven number of arguments")
			}

			out := map[string]any{}

			idx := 0

			for idx+1 < len(args) {
				k := args[idx].(string)

				out[k] = args[idx+1]
				idx += 2
			}

			return out
		},

		// we could do a parameter, but this way we get a free type check
		"markdown_single_post":  markdown(types.ViewSinglePost),
		"markdown_feed":         markdown(types.ViewFeed),
		"markdown_edit_preview": markdown(types.ViewEditPreview),
		"markdown_comment":      markdown(types.ViewComment),
		"markdown_article":      markdown(types.ViewArticle),

		"tzlist": func() []string {
			return util.TimeZones
		},
	}
}

// StaticAssetFunc resolves a frontend asset name to its URL.
type StaticAssetFunc func(n string) string

// LoadStaticManifest reads dist/manifest.json, relative to the process cwd.
func LoadStaticManifest() StaticAssetFunc {
	manifest, err := os.ReadFile("dist/manifest.json")

	if err != nil {
		panic(err)
	}

	files := map[string]string{}

	err = json.Unmarshal(manifest, &files)

	if err != nil {
		panic(err)
	}

	return func(n string) string {
		path, ok := files[n]

		if !ok {
			panic(fmt.Sprintf("asset [%s] is not defined", n))
		}

		prefix := staticRoute

		if pr, ok := os.LookupEnv("STATIC_CDN"); ok && util.InCluster() {
			prefix = pr
		}

		return fmt.Sprintf("%s/%s", prefix, path)
	}
}
