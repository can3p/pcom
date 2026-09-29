package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
