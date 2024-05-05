package validation

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

var usernameRE = regexp.MustCompile(`^[a-z][0-9a-z]*(_[0-9a-z]+)*$`)

func ValidatePassword(p string) error {
	if len(strings.TrimSpace(p)) < 8 {
		return errors.Errorf("Password should be 8 characters or longer")
	}

	return nil
}

func ValidateUsername(p string) error {
	trimmed := strings.ToLower(strings.TrimSpace(p))

	if len(trimmed) < 3 || len(trimmed) > 20 {
		return errors.Errorf("Username should be between 3 and 20 chars")
	}

	if !usernameRE.MatchString(trimmed) {
		return errors.Errorf("Username should start with one or more letters, may contain digits and underscores, cannot have multiple underscores in a row")
	}

	return nil
}

func ValidateMinMax(label string, p string, minL int, maxL int) error {
	trimmed := strings.TrimSpace(p)

	if len(trimmed) < minL || len(trimmed) > maxL {
		return errors.Errorf("%s should be between %d and %d chars", label, minL, maxL)
	}

	return nil
}

func ValidateEnum(value string, vals []string, labels []string) error {
	for _, v := range vals {
		if v == value {
			return nil
		}
	}

	return errors.Errorf("The only allowed options are: %s", strings.Join(labels, ", "))
}
