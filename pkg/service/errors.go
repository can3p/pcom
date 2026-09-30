// Package service holds the errors every service returns. The services
// themselves live in pkg/service/<area>; see docs/architecture.md.
//
// Transports map these errors to responses in one place each:
// ginhelpers.Status for HTML and API pages, actionMessage in pkg/web/app for
// JSON actions.
package service

import "errors"

var (
	// ErrNotFound: the thing doesn't exist, or the actor may not know it
	// exists. Visibility failures on reads return this, not ErrForbidden.
	ErrNotFound = errors.New("not found")
	// ErrForbidden: the actor can see the thing but may not do this to it.
	ErrForbidden = errors.New("forbidden")
	// ErrNeedsLogin: an anonymous actor would be allowed after logging in.
	ErrNeedsLogin = errors.New("needs login")
	// ErrConflict: the change collides with existing state (a taken
	// username, a duplicate invitation). Wrap it with the reason:
	// fmt.Errorf("%w: username is taken", service.ErrConflict).
	ErrConflict = errors.New("conflict")
)

// ValidationError rejects an input. Message is shown to the user as is, so it
// reads as a sentence. Field names the input field, or is empty when the
// error is about the input as a whole.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// Invalid returns a ValidationError for field.
func Invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
