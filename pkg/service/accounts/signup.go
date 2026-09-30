package accounts

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	disposable "github.com/can3p/anti-disposable-email"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

// emailRE and testEmailRE decide what signup accepts as an address.
var (
	emailRE     *regexp.Regexp = validation.EmailRE
	testEmailRE *regexp.Regexp = validation.TestEmailRE
)

// CheckSignupEmail says whether an address may be used to sign up. It
// returns a ValidationError with the reason when it may not.
func (s *Service) CheckSignupEmail(ctx context.Context, address string) error {
	if !emailRE.MatchString(address) {
		return service.Invalid("email", "Invalid email")
	}

	if exists, err := s.store.UserEmailExists(ctx, pgsession.NormalizeEmail(address)); err != nil {
		return err
	} else if exists {
		return service.Invalid("email", "Email is already used in the system")
	}

	if strings.Contains(address, "+") && !testEmailRE.MatchString(address) {
		return service.Invalid("email", "Plus sign is not allowed in the emails")
	}

	parsedEmail, _ := disposable.ParseEmail(address)

	if parsedEmail.Disposable {
		go func() {
			if err := s.send(ctx, s.store, admin.ThrowAwayEmailSignupAttempt(s.ident.From, s.ident.AdminAddress, address)); err != nil {
				log.Printf("failed to queue the throwaway email notification: %v", err)
			}
		}()

		return service.Invalid("email", "Email domain is not allowed, please reach out to us via the support form")
	}

	return nil
}

// CheckWaitingListEmail says whether an address may join the waiting list.
// It returns a ValidationError with the reason when it may not.
func (s *Service) CheckWaitingListEmail(ctx context.Context, address string) error {
	if !emailRE.MatchString(address) {
		return service.Invalid("email", "Invalid email")
	}

	email := pgsession.NormalizeEmail(address)

	if exists, err := s.store.UserEmailExists(ctx, email); err != nil {
		return err
	} else if exists {
		return service.Invalid("email", "Email is already used in the system")
	}

	if exists, err := s.store.InvitationSentTo(ctx, email); err != nil {
		return err
	} else if exists {
		return service.Invalid("email", "Email has been sent already")
	}

	if exists, err := s.store.SignupRequestEmailExists(ctx, email); err != nil {
		return err
	} else if exists {
		return service.Invalid("email", "Email is already in the waiting list")
	}

	return nil
}

// UsernameTaken reports whether an account already has the username.
func (s *Service) UsernameTaken(ctx context.Context, username string) (bool, error) {
	return s.store.UsernameExists(ctx, username)
}

// Signup creates an account that still has to confirm its email address and
// tells the admin about it.
func (s *Service) Signup(ctx context.Context, email, username, password, attribution string) (*core.User, error) {
	var u *core.User

	err := s.store.Tx(ctx, func(tx *repo.Store) (err error) {
		u, err = s.signup(ctx, tx, email, username, password, attribution)

		return err
	})
	if err != nil {
		return nil, err
	}

	return u, nil
}

// Register is what the signup form does: it creates the account, and queues
// the confirmation mail and the admin's notification with it, so that all of
// them exist or none.
func (s *Service) Register(ctx context.Context, email, username, password, attribution string) (*core.User, error) {
	var u *core.User

	err := s.store.Tx(ctx, func(tx *repo.Store) (err error) {
		if u, err = s.signup(ctx, tx, email, username, password, attribution); err != nil {
			return err
		}

		confirm, err := mail.ConfirmSignup(s.ident.Site, s.ident.From, u)
		if err != nil {
			return err
		}

		return fatal(s.send(ctx, tx, confirm))
	})
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (s *Service) signup(ctx context.Context, tx *repo.Store, email, username, password, attribution string) (*core.User, error) {
	if password == "" || email == "" || username == "" {
		return nil, service.Invalid("", "Not enough data")
	}

	u := &core.User{
		ID:                uuid.NewString(),
		Email:             pgsession.NormalizeEmail(email),
		Username:          username,
		Pwdhash:           null.StringFrom(pgsession.HashPassword(password)),
		EmailConfirmSeed:  null.StringFrom(uuid.NewString()),
		SignupAttribution: null.NewString(attribution, attribution != ""),
	}

	if err := tx.InsertUser(ctx, u); err != nil {
		return nil, err
	}

	if err := s.send(ctx, tx, admin.NewUser(s.ident.Site, s.ident.From, s.ident.AdminAddress, u)); err != nil {
		return nil, err
	}

	return u, nil
}

// ConfirmSignup marks the account behind a confirmation link as confirmed
// and tells the admin. Following the link again changes nothing.
func (s *Service) ConfirmSignup(ctx context.Context, seed string) error {
	confirmed := false

	user, err := notFound(s.store.UserBySignupSeed(ctx, seed))
	if err != nil {
		return err
	}

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		if user.EmailConfirmedAt.Valid {
			return nil
		}

		user.EmailConfirmedAt = null.TimeFrom(time.Now())
		confirmed = true

		return tx.SaveUser(ctx, user)
	})
	if err != nil {
		return err
	}

	if confirmed {
		s.notifyAdmin(ctx, "signup confirmed", admin.SignupConfirmed(s.ident.Site, s.ident.From, s.ident.AdminAddress, user))
	}

	return nil
}

// JoinWaitingList adds an address to the waiting list, asks it to confirm
// and tells the admin.
func (s *Service) JoinWaitingList(ctx context.Context, email, reason, attribution string) error {
	request := &core.UserSignupRequest{
		ID:                uuid.NewString(),
		Email:             pgsession.NormalizeEmail(email),
		Reason:            null.NewString(reason, reason != ""),
		SignupAttribution: null.NewString(attribution, attribution != ""),
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.InsertSignupRequest(ctx, request); err != nil {
			return fatal(err)
		}

		request.VerificationSentAt = null.TimeFrom(time.Now())

		if err := tx.SaveSignupRequest(ctx, request); err != nil {
			return fatal(err)
		}

		if err := s.send(ctx, tx, mail.ConfirmWaitingList(s.ident.Site, s.ident.From, request)); err != nil {
			return fatal(err)
		}

		return s.send(ctx, tx, admin.NewWaitingListMember(s.ident.Site, s.ident.From, s.ident.AdminAddress, request))
	})
}

// ConfirmWaitingList marks a waiting list entry as confirmed. Following the
// link again changes nothing.
func (s *Service) ConfirmWaitingList(ctx context.Context, id string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		request, err := notFound(tx.SignupRequestByID(ctx, id))
		if err != nil {
			return err
		}

		if request.EmailConfirmedAt.Valid {
			return nil
		}

		request.EmailConfirmedAt = null.TimeFrom(time.Now())

		return tx.SaveSignupRequest(ctx, request)
	})
}
