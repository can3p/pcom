// Package tommy runs one tommy container (github.com/can3p/tommy) per test
// binary: the Mailjet API mail is sent to, and the S3 bucket user media is
// stored in. Tests read back what the app sent or stored through its API.
//
// Every test shares the container, so a test tells its own mail and objects
// apart by what only it uses: a recipient address, an object key.
package tommy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Image is the tommy release the tests run against.
	Image = "can3p/tommy:0.3.0"
	// Bucket exists in the container from the start.
	Bucket = "pcom-media"
)

// Tommy is a running container.
type Tommy struct {
	// APIURL is the read-back API root, such as http://127.0.0.1:32770/api/v1.
	APIURL string
	// MailjetURL is the Mailjet API root to configure the sender with
	// (MJ_API_BASE).
	MailjetURL string
	// S3URL is the path-style S3 endpoint (USER_MEDIA_ENDPOINT).
	S3URL string

	container *testcontainers.DockerContainer
}

var (
	once    sync.Once
	shared  *Tommy
	errOnce error
)

// Shared starts the container on first use and returns it; later calls in
// the same test binary get the same one. Call Cleanup from TestMain to stop
// it right away; otherwise the testcontainers reaper removes it.
func Shared(t testing.TB) *Tommy {
	t.Helper()

	once.Do(func() { shared, errOnce = start(context.Background()) })

	if errOnce != nil {
		t.Fatalf("tommy: %v", errOnce)
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

func start(ctx context.Context) (*Tommy, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	c, err := testcontainers.Run(ctx, Image,
		testcontainers.WithExposedPorts("8811/tcp", "8822/tcp", "9000/tcp"),
		testcontainers.WithEnv(map[string]string{
			"TOMMY_S3_BUCKETS":      Bucket,
			"TOMMY_NO_UPDATE_CHECK": "1",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/api/v1/health").WithPort("8811/tcp"),
			wait.ForListeningPort("8822/tcp"),
			wait.ForListeningPort("9000/tcp"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("starting %s: %w", Image, err)
	}

	tm := &Tommy{container: c}

	for port, dst := range map[string]*string{"8811/tcp": &tm.APIURL, "8822/tcp": &tm.MailjetURL, "9000/tcp": &tm.S3URL} {
		endpoint, err := c.PortEndpoint(ctx, port, "http")
		if err != nil {
			_ = testcontainers.TerminateContainer(c)
			return nil, fmt.Errorf("resolving port %s: %w", port, err)
		}

		*dst = endpoint
	}

	tm.APIURL += "/api/v1"

	return tm, nil
}
