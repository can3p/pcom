package accounts

import (
	"context"
	"log"
	"regexp"
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

	if taken, err := mailboxTaken(ctx, s.store, pgsession.NormalizeEmail(address)); err != nil {
		return err
	} else if taken {
		return errMailboxTaken
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

var errMailboxTaken = service.Invalid("email", "Email is already used in the system")

// mailboxTaken reports whether an account already uses the mailbox the
// (normalized) email delivers to, so that a +tag or, on Gmail, extra dots
// cannot buy a second account. The owner's test addresses (testEmailRE)
// count only as themselves, so any number of their +tags can sign up.
func mailboxTaken(ctx context.Context, store *repo.Store, email string) (bool, error) {
	if testEmailRE.MatchString(email) {
		return store.UserEmailExists(ctx, email)
	}

	return store.MailboxHasAccount(ctx, email)
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

// Register is what the signup form does: it creates an account that has no
// password and a confirmed email only once its owner types the code, tells the
// admin, starts a login attempt and queues the mail with its code, so that all
// of them exist or none. It returns the attempt's id; finishing it logs the
// user in and confirms the address. A mailbox that already has an account is
// a ValidationError on "email", checked under a lock so that concurrent
// signups cannot each find it free.
func (s *Service) Register(ctx context.Context, email, username, attribution string) (string, error) {
	var attemptID string

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		if email == "" || username == "" {
			return service.Invalid("", "Not enough data")
		}

		u := &core.User{
			ID:                uuid.NewString(),
			Email:             pgsession.NormalizeEmail(email),
			Username:          username,
			SignupAttribution: null.NewString(attribution, attribution != ""),
		}

		if err := tx.LockMailbox(ctx, repo.LockSignupMailbox, u.Email); err != nil {
			return err
		}

		if taken, err := mailboxTaken(ctx, tx, u.Email); err != nil {
			return err
		} else if taken {
			return errMailboxTaken
		}

		if err := tx.InsertUser(ctx, u); err != nil {
			return err
		}

		if err := s.send(ctx, tx, admin.NewUser(s.ident.Site, s.ident.From, s.ident.AdminAddress, u)); err != nil {
			return err
		}

		id, err := s.startAttempt(ctx, tx, u, func(id, code string) *mail.Envelope {
			return mail.ConfirmSignup(s.ident.From, id, u.Email, code, CodeLifetime)
		})
		attemptID = id

		return fatal(err)
	})
	if err != nil {
		return "", err
	}

	return attemptID, nil
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
