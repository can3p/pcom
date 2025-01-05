package validation

import (
	"fmt"
	"net/url"
)

// ValidateURL checks if the provided string is a valid URL.
// Returns nil if the URL is valid or empty, otherwise returns an error with a description.
func ValidateURL(value string) error {
	if value == "" {
		return nil
	}

	_, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}

	return nil
}
