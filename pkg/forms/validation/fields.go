package validation

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var usernameRE = regexp.MustCompile(`^[a-z][0-9a-z]*(_[0-9a-z]+)*$`)

func ValidateUsername(p string) error {
	trimmed := strings.ToLower(strings.TrimSpace(p))

	if len(trimmed) < 3 || len(trimmed) > 20 {
		return errors.New("Username should be between 3 and 20 characters") //nolint:staticcheck // ST1005: message text kept as is
	}

	if !usernameRE.MatchString(trimmed) {
		return errors.New("Username should start with one or more letters, may contain digits and underscores, cannot have multiple underscores in a row") //nolint:staticcheck // ST1005: message text kept as is
	}

	return nil
}

func ValidateMinMax(label string, p string, minL int, maxL int) error {
	trimmed := strings.TrimSpace(p)

	if n := utf8.RuneCountInString(trimmed); n < minL || n > maxL {
		return fmt.Errorf("%s should be between %d and %d characters", label, minL, maxL)
	}

	return nil
}

func ValidateEnum[A comparable](value A, vals []A, labels []string) error {
	if slices.Contains(vals, value) {
		return nil
	}

	return fmt.Errorf("The only allowed options are: %s", strings.Join(labels, ", ")) //nolint:staticcheck // ST1005: message text kept as is
}
