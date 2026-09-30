// Package service holds the errors every service returns. The services
// themselves live in pkg/service/<area>; see docs/architecture.md.
//
// Transports map these errors to responses in one place each:
// gogo ginhelpers.Status for HTML and API pages, actionMessage in pkg/web/app for
// JSON actions.
package service

import "github.com/can3p/gogo/apperr"

var (
	// ErrNotFound: the thing doesn't exist, or the actor may not know it
	// exists. Visibility failures on reads return this, not ErrForbidden.
	ErrNotFound = apperr.ErrNotFound
	// ErrForbidden: the actor can see the thing but may not do this to it.
	ErrForbidden = apperr.ErrForbidden
	// ErrNeedsLogin: an anonymous actor would be allowed after logging in.
	ErrNeedsLogin = apperr.ErrNeedsLogin
	// ErrConflict: the change collides with existing state (a taken
	// username, a duplicate invitation). Wrap it with the reason:
	// fmt.Errorf("%w: username is taken", service.ErrConflict).
	ErrConflict = apperr.ErrConflict
)

// ValidationError rejects an input. Message is shown to the user as is, so it
// reads as a sentence. Field names the input field, or is empty when the
// error is about the input as a whole.
type ValidationError = apperr.ValidationError

// Invalid returns a ValidationError for field.
func Invalid(field, message string) error {
	return apperr.Invalid(field, message)
}
