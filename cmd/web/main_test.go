package main

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// unsetEnv makes sure the named environment variable is not set for the
// duration of the test, restoring whatever value (or absence) it had
// before.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	old, ok := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))

	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(name, old)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

// TestFuncmapParsesAllTemplates is the most valuable single test here: it
// parses every real template with the funcmap this binary registers, using
// a stub static asset function so it never needs `yarn build` output. If a
// template stops parsing (bad func call, typo'd define, ...) this fails.
func TestFuncmapParsesAllTemplates(t *testing.T) {
	stub := func(n string) string {
		return "/static/" + n
	}

	tmpl := template.Must(template.New("").Funcs(funcmap(stub)).ParseGlob("client/html/*.html"))

	require.NotNil(t, tmpl)

	files, err := filepath.Glob("client/html/*.html")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, f := range files {
		name := filepath.Base(f)
		require.NotNil(t, tmpl.Lookup(name), "template %s should be registered", name)
	}
}

func TestFuncmapToMap(t *testing.T) {
	t.Parallel()

	fm := funcmap(func(n string) string { return n })
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

	fm := funcmap(func(n string) string { return n })

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
		// without needing SITE_ROOT or any other env var.
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
	unsetEnv(t, "FLY_APP_NAME")
	unsetEnv(t, "STATIC_CDN")

	dir := t.TempDir()
	writeFakeManifest(t, dir, map[string]string{
		"app.js": "app.abc123.js",
	})

	t.Chdir(dir)

	asset := loadStaticManifest()

	require.Equal(t, "/static/app.abc123.js", asset("app.js"))
}

func TestLoadStaticManifest_CDNOnlyInCluster(t *testing.T) {
	dir := t.TempDir()
	writeFakeManifest(t, dir, map[string]string{
		"app.js": "app.abc123.js",
	})

	t.Chdir(dir)

	t.Setenv("STATIC_CDN", "https://cdn.example.com")

	t.Run("not in cluster: CDN prefix ignored", func(t *testing.T) {
		unsetEnv(t, "FLY_APP_NAME")

		asset := loadStaticManifest()
		require.Equal(t, "/static/app.abc123.js", asset("app.js"))
	})

	t.Run("in cluster: CDN prefix used", func(t *testing.T) {
		t.Setenv("FLY_APP_NAME", "some-app")

		asset := loadStaticManifest()
		require.Equal(t, "https://cdn.example.com/app.abc123.js", asset("app.js"))
	})
}

func TestLoadStaticManifest_UnknownAssetPanics(t *testing.T) {
	unsetEnv(t, "FLY_APP_NAME")

	dir := t.TempDir()
	writeFakeManifest(t, dir, map[string]string{
		"app.js": "app.abc123.js",
	})

	t.Chdir(dir)

	asset := loadStaticManifest()

	require.PanicsWithValue(t, "asset [missing.js] is not defined", func() {
		asset("missing.js")
	})
}

func TestArticlesRE(t *testing.T) {
	t.Parallel()

	valid := []string{"post", "post123", "hello_world", "a_b_c9"}
	invalid := []string{"", "Hello", "-abc", "hello__world", "hello-world", "_hello", "hello_"}

	for _, s := range valid {
		require.True(t, articlesRE.MatchString(s), "expected %q to match", s)
	}

	for _, s := range invalid {
		require.False(t, articlesRE.MatchString(s), "expected %q not to match", s)
	}
}

func TestEnforceEnvVars(t *testing.T) {
	unsetEnv(t, "PCOM_TEST_REQUIRED_VAR")

	require.Panics(t, func() {
		enforceEnvVars([]string{"PCOM_TEST_REQUIRED_VAR"})
	})

	t.Setenv("PCOM_TEST_REQUIRED_VAR", "set")

	require.NotPanics(t, func() {
		enforceEnvVars([]string{"PCOM_TEST_REQUIRED_VAR"})
	})
}

func TestEnforceEnvVars_NoVarsRequired(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		enforceEnvVars(nil)
	})
}
