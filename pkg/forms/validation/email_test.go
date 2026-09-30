package validation_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/stretchr/testify/require"
)

func TestEmailRE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		email string
		valid bool
	}{
		// Valid emails
		{"user@example.com", true},
		{"test.email@example.co.uk", true},
		{"test+tag@example.com", true},
		{"user_name@example.com", true},
		{"123@example.com", true},
		{"user@subdomain.example.com", true},
		{"a@b.co", true},
		{"user@example", true},                    // single-word domain is allowed
		{"user @example.com", true},               // space is allowed in name
		{"user!#$%&'*+/=?^_`{|}~@test.com", true}, // special chars allowed in name
		{"a.b.c@example.com", true},               // dots allowed in name
		{"test'email@example.com", true},          // apostrophe allowed
		{"user123@test456.com", true},             // mixed alphanumeric

		// Invalid emails
		{"invalid", false},
		{"@example.com", false},      // no name part
		{"user@", false},             // no domain
		{"user@@example.com", false}, // double @
		{"", false},                  // empty
		{"user@-example.com", false}, // domain starts with -
		{"user@example.com-", false}, // domain ends with -
		{"user@.example.com", false}, // domain starts with dot
	}

	for _, tc := range testCases {
		t.Run(tc.email, func(t *testing.T) {
			got := validation.EmailRE.MatchString(tc.email)
			require.Equal(t, tc.valid, got, "email: %s, expected: %v, got: %v", tc.email, tc.valid, got)
		})
	}
}

func TestTestEmailRE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		email string
		valid bool
	}{
		// Valid test emails
		{"dpetroff@gmail.com", true},
		{"dpetroff+tag@gmail.com", true},
		{"dpetroff+test+tag@gmail.com", true},

		// Invalid test emails
		{"user@gmail.com", false},
		{"dpetroff@yahoo.com", false},
		{"dpetroff", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.email, func(t *testing.T) {
			got := validation.TestEmailRE.MatchString(tc.email)
			require.Equal(t, tc.valid, got, "email: %s, expected: %v, got: %v", tc.email, tc.valid, got)
		})
	}
}

func TestAttributionRE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value string
		valid bool
	}{
		{"a", true},
		{"index_page", true},
		{"a_b_c", true},
		{"", false},
		{"123", false},
		{"ABC", false},
		{"   ", false},
	}

	for _, tc := range testCases {
		t.Run(tc.value, func(t *testing.T) {
			require.Equal(t, tc.valid, validation.AttributionRE.MatchString(tc.value))
		})
	}
}

func TestAttributionRE_RejectsFreeText(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"Test", "test-attr", "test ", "123test", "Test <b>x</b>"} {
		require.False(t, validation.AttributionRE.MatchString(value), value)
	}
}
