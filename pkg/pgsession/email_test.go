package pgsession_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/stretchr/testify/require"
)

func TestCanonicalEmail(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"user@example.com":           "user@example.com",
		" User@Example.COM ":         "user@example.com",
		"user+promo@example.com":     "user@example.com",
		"user+a+b@example.com":       "user@example.com",
		"first.last@example.com":     "first.last@example.com", // dots count outside Gmail
		"First.Last+x@gmail.com":     "firstlast@gmail.com",
		"first.last@googlemail.com":  "firstlast@gmail.com",
		"user@gmail.com.example.org": "user@gmail.com.example.org",
		"+tag@example.com":           "@example.com",
		"not-an-address":             "not-an-address",
	} {
		require.Equal(t, want, pgsession.CanonicalEmail(in), in)
	}
}
