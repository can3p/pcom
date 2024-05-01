package web

import (
	"context"
	"regexp"
	"strings"

	disposable "github.com/can3p/anti-disposable-email"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var EmailRE *regexp.Regexp = regexp.MustCompile(`(?P<name>[a-zA-Z0-9.!#$%&'*+/=?^_ \x60{|}~-]+)@(?P<domain>[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)$`)
var TestEmailRE *regexp.Regexp = regexp.MustCompile(`dpetroff(\+[^@]+)?@gmail.com`)
var AttributionRE *regexp.Regexp = regexp.MustCompile(`[a-z_]+`)

func EmailOKToAddToWaitingList(ctx context.Context, db boil.ContextExecutor, address string) (string, bool) {
	if !EmailRE.MatchString(address) {
		return "Invalid email", false
	}

	if core.Users(core.UserWhere.Email.EQ(address)).ExistsP(ctx, db) {
		return "Email is already used in the system", false
	}

	if core.UserInvitations(core.UserInvitationWhere.InvitationEmail.EQ(null.StringFrom(address))).ExistsP(ctx, db) {
		return "Email has been sent already", false
	}

	if core.UserSignupRequests(core.UserSignupRequestWhere.Email.EQ(address)).ExistsP(ctx, db) {
		return "Email is already in the waiting list", false
	}

	return "", true
}

func EmailOKToSignup(ctx context.Context, db boil.ContextExecutor, sender sender.Sender, address string) (string, bool) {
	if !EmailRE.MatchString(address) {
		return "Invalid email", false
	}

	if core.Users(core.UserWhere.Email.EQ(address)).ExistsP(ctx, db) {
		return "Email is already used in the system", false
	}

	if strings.Contains(address, "+") && !TestEmailRE.MatchString(address) {
		return "Plus sign is not allowed in the emails", false
	}

	parsedEmail, _ := disposable.ParseEmail(address)

	if parsedEmail.Disposable {
		go admin.NotifyThrowAwayEmailSignupAttempt(ctx, sender, address)

		return "Email domain is not allowed, please reach out to us via the support form", false
	}

	return "", true
}
