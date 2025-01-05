package validation

import (
	"fmt"
	"net/url"
)

// ValidateURL checks if the provided string is a valid URL.
// The URL must be absolute (have a protocol) and include a domain name.
// Returns nil if the URL is valid or empty, otherwise returns an error with a description.
func ValidateURL(value string) error {
	if value == "" {
		return nil
	}

	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}

	if !u.IsAbs() {
		return fmt.Errorf("URL must include a protocol (e.g., https://)")
	}

	if u.Host == "" {
		return fmt.Errorf("URL must include a domain name")
	}

	return nil
}
