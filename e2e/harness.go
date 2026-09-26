// Package e2e runs the real web binary against a fresh database and drives it
// over HTTP. Tests only see what a browser would see (status codes, headers,
// HTML), so they keep passing while the code behind the routes is refactored.
//
// Every test package that uses Start needs a TestMain that calls Main:
//
//	func TestMain(m *testing.M) { e2e.Main(m) }
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/can3p/gogo/testcontainers/postgres"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
)

var (
	// repoRoot is the module root, resolved from this file.
	repoRoot = func() string {
		_, file, _, _ := runtime.Caller(0)
		return filepath.Dir(filepath.Dir(file))
	}()

	// binPath is the web binary built by Main; empty when Main didn't build it.
	binPath string
)

// Main builds the web binary once for the test package, runs the tests and
// cleans up. With -short it builds nothing, and Start skips the test.
func Main(m *testing.M) {
	os.Exit(Run(m))
}

// Run is Main without the exit: it returns the exit code, for a TestMain
// that has its own setup and teardown around the tests.
func Run(m *testing.M) int {
	flag.Parse()

	if testing.Short() {
		return m.Run()
	}

	dir, err := os.MkdirTemp("", "pcom-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	binPath, err = buildBinary(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}

	code := m.Run()

	_ = postgres.Cleanup()

	return code
}

// buildBinary builds ./cmd/web with coverage instrumentation. An overlay adds
// a SIGTERM handler (testdata/exit_on_sigterm.go), so a stopped server exits
// normally and writes its coverage data to GOCOVERDIR.
func buildBinary(dir string) (string, error) {
	const hookPkg = "github.com/can3p/pcom/pkg/types"

	list := exec.Command("go", "list", "./...")
	list.Dir = repoRoot

	pkgs, err := list.Output()
	if err != nil {
		return "", fmt.Errorf("listing packages: %w", err)
	}

	var coverPkgs []string
	for p := range strings.FieldsSeq(string(pkgs)) {
		if p != hookPkg {
			coverPkgs = append(coverPkgs, p)
		}
	}

	overlay := map[string]map[string]string{"Replace": {
		filepath.Join(repoRoot, "pkg", "types", "zz_e2e_exit_on_sigterm.go"): filepath.Join(repoRoot, "e2e", "testdata", "exit_on_sigterm.go"),
	}}

	overlayJSON, err := json.Marshal(overlay)
	if err != nil {
		return "", err
	}

	overlayPath := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayJSON, 0o600); err != nil {
		return "", err
	}

	bin := filepath.Join(dir, "web")
	cmd := exec.Command("go", "build", "-cover", "-coverpkg", strings.Join(coverPkgs, ","),
		"-overlay", overlayPath, "-o", bin, "./cmd/web")
	cmd.Dir = repoRoot

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building ./cmd/web: %w\n%s", err, out)
	}

	return bin, nil
}

// App is one running instance of the web binary with its own database.
type App struct {
	// URL is the server's root, such as http://127.0.0.1:41234, without a
	// trailing slash.
	URL string
	// DB is the app's database, for creating fixtures with the factories and
	// asserting on rows.
	DB *sqlx.DB
}

// Option configures Start.
type Option func(*config)

type config struct {
	env        map[string]string
	realAssets bool
}

// WithEnv sets an extra environment variable for the binary.
func WithEnv(key, value string) Option {
	return func(c *config) { c.env[key] = value }
}

// WithRealAssets serves the frontend build in cmd/web/dist instead of the
// stub assets, for tests that run the page's JavaScript and styles in a
// browser. The build must exist: `make test-ui` runs `yarn build` first.
func WithRealAssets() Option {
	return func(c *config) { c.realAssets = true }
}

// Start runs the web binary against a fresh database and returns once it
// serves GET / with 200. The process is stopped when the test ends, and its
// output is logged if the test failed.
func Start(t testing.TB, opts ...Option) *App {
	t.Helper()

	if binPath == "" {
		if testing.Short() {
			t.Skip("e2e: skipped in -short mode")
		}

		t.Fatal("e2e: the web binary was not built; call e2e.Main from TestMain")
	}

	cfg := config{env: map[string]string{}}
	for _, o := range opts {
		o(&cfg)
	}

	db := testdb.New(t)
	work := workDir(t, cfg.realAssets)
	port := freePort(t)
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	env := map[string]string{
		"PORT":         fmt.Sprint(port),
		"DATABASE_URL": db.URL,
		"SESSION_SALT": "test",
		"SITE_ROOT":    url,
		"GIN_MODE":     "release",
	}
	if dir := coverDir(); dir != "" {
		env["GOCOVERDIR"] = dir
	}
	maps.Copy(env, cfg.env)

	cmd := exec.Command(binPath)
	cmd.Dir = work
	cmd.Env = processEnv(env)

	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		t.Fatalf("e2e: starting the web binary: %v", err)
	}

	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	t.Cleanup(func() {
		stop(cmd, exited)

		if t.Failed() {
			t.Logf("e2e: web binary output:\n%s", out.String())
		}
	})

	waitReady(t, url, exited, out)

	return &App{URL: url, DB: db.DB}
}

// coverDir is where the binary writes coverage data: the test's own
// -test.gocoverdir under `go test -cover` (which is what `make cover`
// passes), or else GOCOVERDIR from the environment.
func coverDir() string {
	if f := flag.Lookup("test.gocoverdir"); f != nil && f.Value.String() != "" {
		return f.Value.String()
	}

	return os.Getenv("GOCOVERDIR")
}

// processEnv is the test's environment with the given overrides. FLY_APP_NAME
// is removed, because it switches the binary to production mode.
func processEnv(overrides map[string]string) []string {
	var env []string

	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if _, ok := overrides[key]; ok || key == "FLY_APP_NAME" {
			continue
		}

		env = append(env, kv)
	}

	for k, v := range overrides {
		env = append(env, k+"="+v)
	}

	return env
}

// workDir makes the binary's working directory: templates and articles
// through a `client` symlink, and either the real `dist` through a symlink or
// a stub `dist` whose manifest names every asset the templates ask for,
// because static_asset panics on unknown keys.
func workDir(t testing.TB, realAssets bool) string {
	t.Helper()

	work := t.TempDir()

	if err := os.Symlink(filepath.Join(repoRoot, "cmd", "web", "client"), filepath.Join(work, "client")); err != nil {
		t.Fatal(err)
	}

	if realAssets {
		dist := filepath.Join(repoRoot, "cmd", "web", "dist")
		if _, err := os.Stat(filepath.Join(dist, "manifest.json")); err != nil {
			t.Fatalf("e2e: no frontend build in %s; run `yarn build` in cmd/web (make test-ui does): %v", dist, err)
		}

		if err := os.Symlink(dist, filepath.Join(work, "dist")); err != nil {
			t.Fatal(err)
		}

		return work
	}

	keys, err := templateAssets(filepath.Join(repoRoot, "cmd", "web", "client", "html"))
	if err != nil {
		t.Fatal(err)
	}

	manifest := map[string]string{}

	for _, key := range keys {
		manifest[key] = key

		path := filepath.Join(work, "dist", filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("/* e2e stub */\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(work, "dist", "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	return work
}

var staticAssetRE = regexp.MustCompile(`static_asset\s+"([^"]+)"`)

// templateAssets returns every key passed to static_asset in the templates.
func templateAssets(dir string) ([]string, error) {
	seen := map[string]bool{}
	var keys []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, m := range staticAssetRE.FindAllStringSubmatch(string(data), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				keys = append(keys, m[1])
			}
		}

		return nil
	})

	if err == nil && len(keys) == 0 {
		err = errors.New("no static_asset keys found in the templates")
	}

	return keys, err
}

func freePort(t testing.TB) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	return l.Addr().(*net.TCPAddr).Port
}

func waitReady(t testing.TB, url string, exited <-chan struct{}, out *syncBuffer) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		select {
		case <-exited:
			t.Fatalf("e2e: the web binary exited during startup:\n%s", out.String())
		case <-ctx.Done():
			t.Fatalf("e2e: the web binary did not serve GET / with 200 within 30s (last error: %v):\n%s", err, out.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// stop sends SIGTERM, so a coverage build writes its data, and kills the
// process if it hasn't exited after a few seconds.
func stop(cmd *exec.Cmd, exited <-chan struct{}) {
	_ = cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
}

// syncBuffer collects the process output, written from exec's copying
// goroutines and read by the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
