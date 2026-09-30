package mail

import (
	"github.com/badoux/checkmail"
	"github.com/pkg/errors"
)

// ValidateFormat checks that email looks like an email address.
func ValidateFormat(email string) error {
	if err := checkmail.ValidateFormat(email); err != nil {
		return errors.Errorf("Invalid email format")
	}

	return nil
}
