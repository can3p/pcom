package forms

import (
	"context"
	"errors"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/gin-gonic/gin"
)

// SubscriptionsLostFormInput is the form of a subscriber who lost the link to
// their subscriptions: an address, and the guard's fields.
type SubscriptionsLostFormInput struct {
	Email string `form:"email"`
	abuseFields
}

// SubscriptionsLostForm mails an address the link to manage its
// subscriptions. Like SubscribeForm it answers the same whether the address
// subscribes to anyone or not.
type SubscriptionsLostForm struct {
	*forms.FormBase[SubscriptionsLostFormInput]
	Subscriptions *subscriptions.Service
	Guard         AbuseGuard
}

// SubscriptionsLostFormNew builds the form, with a fresh challenge.
func SubscriptionsLostFormNew(svc *subscriptions.Service, guard AbuseGuard) forms.Form {
	var form forms.Form = &SubscriptionsLostForm{
		FormBase: &forms.FormBase[SubscriptionsLostFormInput]{
			Name:         "subscriptions_lost",
			FormTemplate: "partial--subscriptions-lost-form",
			Input:        &SubscriptionsLostFormInput{},
			ExtraTemplateData: map[string]any{
				"Challenge": guard.Issue().Token,
			},
		},
		Subscriptions: svc,
		Guard:         guard,
	}

	return form
}

func (f *SubscriptionsLostForm) Validate(*gin.Context) error {
	f.Input.Email = strings.TrimSpace(f.Input.Email)

	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	}

	return f.Errors.PassedValidation()
}

func (f *SubscriptionsLostForm) Save(ctx context.Context) (forms.FormSaveAction, error) {
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
		err = f.Subscriptions.RequestManageLink(ctx, f.Input.Email)
	}

	return saved(f, &f.FormSaved, err)
}
