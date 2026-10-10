package mail

import (
	"errors"
	"github.com/badoux/checkmail"
)

// ValidateFormat checks that email looks like an email address.
func ValidateFormat(email string) error {
	if err := checkmail.ValidateFormat(email); err != nil {
		return errors.New("Invalid email format") //nolint:staticcheck // ST1005: message text kept as is
	}

	return nil
}
