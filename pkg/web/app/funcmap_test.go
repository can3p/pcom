package app

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/stretchr/testify/require"
)

// templatesGlob is the binary's template directory, seen from this package.
const templatesGlob = "../../../cmd/web/client/html/*.html"

// TestFuncmapParsesAllTemplates is the most valuable single test here: it
// parses every real template with the funcmap this binary registers, using
// a stub static asset function so it never needs `yarn build` output. If a
// template stops parsing (bad func call, typo'd define, ...) this fails.
func TestFuncmapParsesAllTemplates(t *testing.T) {
	stub := func(n string) string {
		return "/static/" + n
	}

	tmpl := template.Must(template.New("").Funcs(funcmap(stub, links.Site{})).ParseGlob(templatesGlob))

	require.NotNil(t, tmpl)

	files, err := filepath.Glob(templatesGlob)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, f := range files {
		name := filepath.Base(f)
		require.NotNil(t, tmpl.Lookup(name), "template %s should be registered", name)
	}
}

func TestFuncmapToMap(t *testing.T) {
	t.Parallel()

	fm := funcmap(func(n string) string { return n }, links.Site{})
	toMap := fm["toMap"].(func(args ...any) map[string]any)

	t.Run("even args", func(t *testing.T) {
		got := toMap("a", 1, "b", "two")
		require.Equal(t, map[string]any{"a": 1, "b": "two"}, got)
	})

	t.Run("no args", func(t *testing.T) {
		got := toMap()
		require.Equal(t, map[string]any{}, got)
	})

	t.Run("odd args panic", func(t *testing.T) {
		require.PanicsWithValue(t, "toMap got uneven number of arguments", func() {
			toMap("a", 1, "b")
		})
	})
}

func TestFuncmapMarkdownFuncsRegistered(t *testing.T) {
	t.Parallel()

	fm := funcmap(func(n string) string { return n }, links.Site{})

	names := []string{
		"markdown_single_post",
		"markdown_feed",
		"markdown_edit_preview",
		"markdown_comment",
		"markdown_article",
	}

	for _, name := range names {
		fn, ok := fm[name].(func(s string, add ...string) template.HTML)
		require.True(t, ok, "funcmap should register %s with the expected signature", name)

		// plain text has no links or media, so this exercises the function
		// without needing a site root.
		out := fn("hello world")
		require.Contains(t, string(out), "hello world")
	}
}

func writeFakeManifest(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	distDir := filepath.Join(dir, "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))

	body, err := json.Marshal(files)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(distDir, "manifest.json"), body, 0o644))
}

func TestLoadStaticManifest(t *testing.T) {
	dir := t.TempDir()
	writeFakeManifest(t, dir, map[string]string{
		"app.js": "app.abc123.js",
	})

	t.Chdir(dir)

	asset := LoadStaticManifest("")

	require.Equal(t, "/static/app.abc123.js", asset("app.js"))
}

func TestLoadStaticManifest_CDN(t *testing.T) {
	dir := t.TempDir()
	writeFakeManifest(t, dir, map[string]string{
		"app.js": "app.abc123.js",
	})

	t.Chdir(dir)

	t.Run("CDN set: prefix used", func(t *testing.T) {
		asset := LoadStaticManifest("https://cdn.example.com")
		require.Equal(t, "https://cdn.example.com/app.abc123.js", asset("app.js"))
	})

	t.Run("CDN empty: served from /static", func(t *testing.T) {
		asset := LoadStaticManifest("")
		require.Equal(t, "/static/app.abc123.js", asset("app.js"))
	})
}

func TestLoadStaticManifest_UnknownAssetPanics(t *testing.T) {
	dir := t.TempDir()
	writeFakeManifest(t, dir, map[string]string{
		"app.js": "app.abc123.js",
	})

	t.Chdir(dir)

	asset := LoadStaticManifest("")

	require.PanicsWithValue(t, "asset [missing.js] is not defined", func() {
		asset("missing.js")
	})
}
