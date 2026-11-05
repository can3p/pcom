package forms

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/gin-gonic/gin"
)

// RetryMessage is what a visitor reads when the guard refused the proof of
// work: the form comes back with a fresh challenge.
const RetryMessage = "Something went wrong, please try again."

// AbuseGuard is what the public forms that mail an address need of
// pkg/abuse's guard: a challenge for the form to carry, and the check of what
// came back.
type AbuseGuard interface {
	Issue() abuse.Challenge
	Verify(ctx context.Context, sub abuse.Submission) error
}

// abuseFields are the fields the guard checks, which every form it protects
// carries; their names are abuse.PowChallengeField, abuse.PowNonceField and
// abuse.HoneypotField.
type abuseFields struct {
	PowChallenge string `form:"pow_challenge"`
	PowNonce     string `form:"pow_nonce"`
	Honeypot     string `form:"website"`
}

func (a abuseFields) submission() abuse.Submission {
	return abuse.Submission{Token: a.PowChallenge, Nonce: a.PowNonce, Honeypot: a.Honeypot}
}

var errRetry = errors.New("the proof of work was refused")

// verifyHuman asks the guard about a submission. silent is a script's
// submission, to answer as if it succeeded; a refused proof of work is
// errRetry.
func verifyHuman(ctx context.Context, guard AbuseGuard, in abuseFields) (silent bool, err error) {
	switch err := guard.Verify(ctx, in.submission()); {
	case err == nil:
		return false, nil
	case errors.Is(err, abuse.ErrSilent):
		return true, nil
	case errors.Is(err, abuse.ErrInvalid):
		return false, errRetry
	default:
		return false, err
	}
}

// saved is the end of a protected form's Save: err, the service's answer (nil
// for a script's submission, which got no answer), becomes the form's
// error, a not found author, or the same reply every success gets.
func saved(f interface {
	AddError(field, message string)
	SetFormError(message string)
}, formSaved *bool, err error) (forms.FormSaveAction, error) {
	if invalid, ok := errors.AsType[*service.ValidationError](err); ok {
		if invalid.Field == "email" {
			f.AddError("email", invalid.Message)
		} else {
			f.SetFormError(invalid.Message)
		}

		return forms.FormSaveDefault(true), nil
	}

	if errors.Is(err, service.ErrNotFound) {
		return func(c *gin.Context, _ forms.Form) { c.Status(http.StatusNotFound) }, nil
	}

	if err != nil {
		return nil, err
	}

	*formSaved = true

	return forms.FormSaveDefault(false), nil
}

// SubscribeFormInput is the subscribe form: an address, and the guard's
// fields.
type SubscribeFormInput struct {
	Email string `form:"email"`
	abuseFields
}

// SubscribeForm subscribes an address to an author's public posts. It answers
// the same for every state of the address, and a script's submission as if
// it had worked, so no answer tells who subscribes to whom.
type SubscribeForm struct {
	*forms.FormBase[SubscribeFormInput]
	Subscriptions *subscriptions.Service
	Guard         AbuseGuard
	// Author is the username of the journal the form is on.
	Author string
}

// SubscribeFormNew builds the form for author's journal. Each form carries a
// fresh challenge, so a form shown again after an error can be answered.
func SubscribeFormNew(svc *subscriptions.Service, guard AbuseGuard, author string) forms.Form {
	var form forms.Form = &SubscribeForm{
		FormBase: &forms.FormBase[SubscribeFormInput]{
			Name:         "subscribe",
			FormTemplate: "partial--subscribe.html",
			Input:        &SubscribeFormInput{},
			ExtraTemplateData: map[string]any{
				"Username":  author,
				"Challenge": guard.Issue().Token,
			},
		},
		Subscriptions: svc,
		Guard:         guard,
		Author:        author,
	}

	return form
}

func (f *SubscribeForm) Validate(*gin.Context) error {
	f.Input.Email = strings.TrimSpace(f.Input.Email)

	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	}

	return f.Errors.PassedValidation()
}

func (f *SubscribeForm) Save(ctx context.Context) (forms.FormSaveAction, error) {
	ctx = ctx.(*gin.Context).Request.Context()

	silent, err := verifyHuman(ctx, f.Guard, f.Input.abuseFields)
	if errors.Is(err, errRetry) {
		f.SetFormError(RetryMessage)

		return forms.FormSaveDefault(true), nil
	}

	if err != nil {
		return nil, err
	}

	if !silent {
		err = f.Subscriptions.Subscribe(ctx, f.Author, f.Input.Email)
	}

	return saved(f, &f.FormSaved, err)
}
