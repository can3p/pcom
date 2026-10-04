package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// userStyleHooks are the classes user stylesheets target. They are a public
// contract: restyling a page may move them, but never drop them.
var userStyleHooks = []string{
	"us-user-home", "us-user-header", "us-profile-about",
	"us-feed-post", "us-feed-post-stats", "us-feed-comment", "us-feed-rss-item",
	"us-post-date", "us-load-more",
	"us-single-post", "us-post-header", "us-post-body", "us-public-link",
	"us-comment-stats", "us-comments-section", "us-single-comment", "us-comment-form",
}

func TestTemplatesKeepUserStyleHooks(t *testing.T) {
	files, err := filepath.Glob(templatesGlob)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var all strings.Builder

	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		all.Write(b)
		all.WriteByte('\n')
	}

	src := all.String()

	for _, hook := range userStyleHooks {
		// a whole class name: us-feed-post must not pass on us-feed-post-stats alone
		re := regexp.MustCompile(`(?:^|[\s"'])` + regexp.QuoteMeta(hook) + `(?:[\s"']|$)`)
		require.True(t, re.MatchString(src), "user style hook %q is missing from the templates", hook)
	}
}
