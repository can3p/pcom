package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// bootstrapClass matches class names that only Bootstrap defined. pcom's own
// components (.btn, .btn-primary, .btn-secondary, .btn-danger, .nav, .toast)
// are not on the list. Bootstrap is no longer loaded, so such a class styles
// nothing and means a template was left half migrated.
var bootstrapClass = regexp.MustCompile(`^(?:btn-(?:sm|lg|outline-.*|link|close|light|dark|info|warning|success)|` +
	`card(?:-.*)?|col(?:-.*)?|row|container(?:-.*)?|form-(?:control|label|select|check.*|text)|alert(?:-.*)?|` +
	`d-.*|[mp][tbsexy]?-\d+|text-muted|fs-\d+|navbar(?:-.*)?|bi|bi-.*|badge|list-group.*|dropdown.*|modal.*|` +
	`visually-hidden|justify-content-.*|align-items-.*|flex-.*)$`)

var (
	classAttr     = regexp.MustCompile(`class="([^"{]*)"`)
	jsClassString = regexp.MustCompile(`classList\.(?:add|remove|toggle|contains)\(([^)]*)\)|className\s*=\s*("[^"]*")`)
	quoted        = regexp.MustCompile(`"([^"]*)"`)
)

func TestTemplatesAndControllersUseNoBootstrapClasses(t *testing.T) {
	check := func(file string, names string) {
		for c := range strings.FieldsSeq(names) {
			require.False(t, bootstrapClass.MatchString(c) || strings.HasPrefix(c, "data-bs-"),
				"%s uses the Bootstrap class %q", file, c)
		}
	}

	templates, err := filepath.Glob(templatesGlob)
	require.NoError(t, err)
	require.NotEmpty(t, templates)

	for _, f := range templates {
		b, err := os.ReadFile(f)
		require.NoError(t, err)

		for _, m := range classAttr.FindAllStringSubmatch(string(b), -1) {
			check(f, m[1])
		}

		require.NotContains(t, string(b), "data-bs-", f)
	}

	controllers, err := filepath.Glob("../../../cmd/web/client/js/controllers/*.js")
	require.NoError(t, err)
	require.NotEmpty(t, controllers)

	for _, f := range controllers {
		b, err := os.ReadFile(f)
		require.NoError(t, err)

		for _, m := range jsClassString.FindAllStringSubmatch(string(b), -1) {
			for _, q := range quoted.FindAllStringSubmatch(m[1]+m[2], -1) {
				check(f, q[1])
			}
		}
	}
}
