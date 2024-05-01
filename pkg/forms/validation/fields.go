package validation

import (
	"strings"

	"github.com/pkg/errors"
)

func ValidatePassword(p string) error {
	if len(strings.TrimSpace(p)) < 8 {
		return errors.Errorf("Password should be 8 characters or longer")
	}

	return nil
}
