package forms

import (
	"errors"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/service"
	"github.com/gin-gonic/gin"
)

// What a form shows of a service's refusal. A form's own checks stay in its
// Validate; these show what the service enforces on the form, so the user can
// fix it (see "Forms" in docs/architecture.md).

// showInvalid shows a service's ValidationError as a field error: under
// field, or under the error's own field when field is "". It reports whether
// err was one.
func showInvalid(f interface{ AddError(field, message string) }, field string, err error) bool {
	invalid, ok := errors.AsType[*service.ValidationError](err)
	if !ok {
		return false
	}

	if field == "" {
		field = invalid.Field
	}

	f.AddError(field, invalid.Message)

	return true
}

// fieldError is for a service check called from Validate: a ValidationError
// shows under field; any other error is a failing database, a bug and not
// the user's to fix, so it panics.
func fieldError(f interface{ AddError(field, message string) }, field string, err error) error {
	if err == nil || showInvalid(f, field, err) {
		return nil
	}

	panic(err)
}

// refused is for a service write called from Save: it draws the form again
// with a ValidationError under its field, or with something that isn't the
// user's (or is gone) as the form's error. Any other error is returned, which
// DefaultHandler answers with 500.
func refused(f forms.Form, err error) (forms.FormSaveAction, error) {
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrForbidden):
		f.SetFormError("Not found.")
	case !showInvalid(f, "", err):
		return nil, err
	}

	return func(c *gin.Context, f forms.Form) { f.RenderForm(c) }, nil
}
