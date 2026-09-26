// Package golden compares test output against a checked-in file, the
// convention docs/testing.md uses for mail content: run with UPDATE_GOLDEN=1
// to write the file, and without it to check the current output still
// matches.
package golden

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/can3p/pcom/pkg/testutil"
)

// updateEnvVar is the environment variable that switches Assert from
// comparing to writing, per docs/testing.md's convention:
//
//	UPDATE_GOLDEN=1 go test ./pkg/mail/...
const updateEnvVar = "UPDATE_GOLDEN"

// Assert compares got against the golden file testdata/<name>.golden.
//
// With UPDATE_GOLDEN set to anything non-empty, it writes got as the new
// golden file (creating testdata/ if needed) instead of comparing, and a
// test that only calls Assert cannot fail. Otherwise it fails the test when
// the file is missing or its content differs from got.
func Assert(t testutil.TB, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", name+".golden")

	if os.Getenv(updateEnvVar) != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("golden: creating %s: %v", filepath.Dir(path), err)
			return
		}

		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("golden: writing %s: %v", path, err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden: reading %s: %v (run with %s=1 to create it)", path, err, updateEnvVar)
		return
	}

	if !bytes.Equal(want, got) {
		t.Fatalf("golden: %s does not match; run with %s=1 to update it\n--- want ---\n%s\n--- got ---\n%s",
			path, updateEnvVar, want, got)
	}
}
