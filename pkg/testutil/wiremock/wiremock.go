// Package wiremock runs one WireMock container per test binary. It stands in
// for the third-party APIs pcom calls for an answer (the first is Azure
// Translator); tommy stays for mail and S3.
//
// Stubs are JSON mapping files next to the code they serve, such as
// pkg/translate/azure/testdata/wiremock/mappings/*.json, with long bodies in
// a sibling __files directory. A package names its directory with Register;
// the container imports every registered directory when it starts. Each API
// lives under its own path prefix (the urlPath in its mappings starts with
// it), and a backend's endpoint is URL(prefix).
//
// Every test shares the container, so a test uses content only it sends and
// verifies calls by matching that content. Watch makes an unmatched call to
// the test's prefix fail the test.
package wiremock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	wm "github.com/wiremock/go-wiremock"
)

// Image is the WireMock release the tests run against.
const Image = "wiremock/wiremock:3.13.2"

var (
	regMu sync.Mutex
	dirs  []string
)

// Register names a directory holding WireMock's mappings/ (and optionally
// __files/) for the container to import at start. Call it before Shared, from
// TestMain or a package-level var.
func Register(dir string) {
	regMu.Lock()
	defer regMu.Unlock()

	dirs = append(dirs, dir)
}

// WireMock is a running container.
type WireMock struct {
	// BaseURL is the container root, such as http://127.0.0.1:32770.
	BaseURL string

	container *testcontainers.DockerContainer
	client    *wm.Client
}

var (
	once    sync.Once
	shared  *WireMock
	errOnce error
)

// Shared starts the container on first use and returns it; later calls in
// the same test binary get the same one. Call Cleanup from TestMain to stop
// it right away; otherwise the testcontainers reaper removes it.
func Shared(t testing.TB) *WireMock {
	t.Helper()

	once.Do(func() { shared, errOnce = start(context.Background()) })

	if errOnce != nil {
		t.Fatalf("wiremock: %v", errOnce)
	}

	return shared
}

// Cleanup stops the shared container, if one was started.
func Cleanup() error {
	if shared == nil {
		return nil
	}

	return testcontainers.TerminateContainer(shared.container)
}

// URL is the endpoint of the API mounted under prefix, e.g. "/azure-translator".
func (w *WireMock) URL(prefix string) string {
	return w.BaseURL + "/" + strings.Trim(prefix, "/")
}

// Client is go-wiremock's client for the admin API.
func (w *WireMock) Client() *wm.Client { return w.client }

// Verify fails the test unless the container received exactly want requests
// that match the pattern. Match on content only the test sends.
func (w *WireMock) Verify(t testing.TB, want int64, pattern *wm.Request) {
	t.Helper()

	got, err := w.client.GetCountRequests(pattern)
	if err != nil {
		t.Fatalf("wiremock: counting requests: %v", err)
	}

	if got != want {
		t.Errorf("wiremock: %d requests matched, want %d", got, want)
	}
}

// Watch makes the test fail, when it ends, for every request to prefix that
// matched no stub, and reports the near misses. Other prefixes are ignored,
// so parallel tests don't fail each other. It returns URL(prefix).
func (w *WireMock) Watch(t testing.TB, prefix string) string {
	t.Helper()

	t.Cleanup(func() { w.reportUnmatched(t, "/"+strings.Trim(prefix, "/")) })

	return w.URL(prefix)
}

func (w *WireMock) reportUnmatched(t testing.TB, prefix string) {
	t.Helper()

	var near struct {
		NearMisses []struct {
			Request struct {
				Method string `json:"method"`
				URL    string `json:"url"`
				Body   string `json:"body"`
			} `json:"request"`
			MatchResult struct {
				Distance float64 `json:"distance"`
			} `json:"matchResult"`
			StubMapping struct {
				Request json.RawMessage `json:"request"`
			} `json:"stubMapping"`
		} `json:"nearMisses"`
	}

	if err := w.admin(http.MethodGet, "/requests/unmatched/near-misses", nil, &near); err != nil {
		t.Errorf("wiremock: reading unmatched requests: %v", err)
		return
	}

	for _, n := range near.NearMisses {
		if !strings.HasPrefix(n.Request.URL, prefix+"/") && n.Request.URL != prefix {
			continue
		}

		t.Errorf("wiremock: unmatched %s %s body %q (distance %.2f to stub %s)",
			n.Request.Method, n.Request.URL, n.Request.Body, n.MatchResult.Distance, n.StubMapping.Request)
	}
}

func start(ctx context.Context) (*WireMock, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	c, err := testcontainers.Run(ctx, Image,
		testcontainers.WithExposedPorts("8080/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/__admin/health").WithPort("8080/tcp")),
	)
	if err != nil {
		return nil, fmt.Errorf("starting %s: %w", Image, err)
	}

	endpoint, err := c.PortEndpoint(ctx, "8080/tcp", "http")
	if err != nil {
		_ = testcontainers.TerminateContainer(c)
		return nil, fmt.Errorf("resolving port: %w", err)
	}

	w := &WireMock{BaseURL: endpoint, container: c, client: wm.NewClient(endpoint)}

	regMu.Lock()
	registered := append([]string(nil), dirs...)
	regMu.Unlock()

	for _, dir := range registered {
		if err := w.importDir(dir); err != nil {
			_ = testcontainers.TerminateContainer(c)
			return nil, fmt.Errorf("importing %s: %w", dir, err)
		}
	}

	return w, nil
}

// importDir sends every file of dir/mappings through the admin import API.
// A mapping file holds one stub or a {"mappings": [...]} list. A response's
// bodyFileName is read from dir/__files and inlined, so no volume is needed.
func (w *WireMock) importDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "mappings", "*.json"))
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no mappings in %s", filepath.Join(dir, "mappings"))
	}

	var stubs []map[string]any

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}

		var doc struct {
			Mappings []map[string]any `json:"mappings"`
		}

		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}

		if doc.Mappings == nil {
			var one map[string]any
			if err := json.Unmarshal(raw, &one); err != nil {
				return fmt.Errorf("%s: %w", f, err)
			}

			doc.Mappings = []map[string]any{one}
		}

		stubs = append(stubs, doc.Mappings...)
	}

	for _, s := range stubs {
		resp, _ := s["response"].(map[string]any)

		name, _ := resp["bodyFileName"].(string)
		if name == "" {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, "__files", name))
		if err != nil {
			return err
		}

		delete(resp, "bodyFileName")
		resp["body"] = string(body)
	}

	return w.admin(http.MethodPost, "/mappings/import", map[string]any{"mappings": stubs}, nil)
}

func (w *WireMock) admin(method, path string, in, out any) error {
	var body io.Reader

	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}

		body = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, w.BaseURL+"/__admin"+path, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, data)
	}

	if out != nil {
		return json.Unmarshal(data, out)
	}

	return nil
}
