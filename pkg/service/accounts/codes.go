package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

const (
	// CodeLifetime is how long a login code works.
	CodeLifetime = 15 * time.Minute
	// codeDigits is the length of a code: 10^8 of them leave a guesser who
	// gets maxWrongTries a wrongTryWindow nothing to hope for.
	codeDigits = 8
	// maxCodeTries is how many tries an attempt allows; after that it is dead.
	maxCodeTries = 5
	// maxCodesMailed codes are mailed to one user per codeLimitWindow.
	maxCodesMailed  = 3
	codeLimitWindow = 15 * time.Minute
	// After maxWrongTries wrong codes on a user's attempts started within
	// wrongTryWindow, no code logs the user in and none is mailed until the
	// window moves past them.
	maxWrongTries  = 10
	wrongTryWindow = time.Hour
)

var errNoCodeKey = errors.New("accounts: login codes need a code key (accounts.WithCodeKey)")

// clock is the service's current time in UTC, the zone timestamps are
// stored in, at the database's microsecond precision, so a time read back
// compares equal to the one written.
func (s *Service) clock() time.Time {
	return s.now().UTC().Truncate(time.Microsecond)
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(math.Pow10(codeDigits))))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}

// codeHash binds a code to its attempt, so a code is useless on any other.
func (s *Service) codeHash(attemptID, code string) string {
	mac := hmac.New(sha256.New, s.codeKey)
	mac.Write([]byte(attemptID + ":" + code))

	return hex.EncodeToString(mac.Sum(nil))
}

// open reports whether an attempt still takes tries at now. An attempt with
// no user takes them too, so it answers like any other until it is used up;
// it just never matches.
func open(a *core.LoginAttempt, now time.Time) bool {
	return !a.UsedAt.Valid && a.ExpiresAt.After(now) && a.Tries < maxCodeTries
}

// lockLoginCodes serializes everything that counts a user's codes and tries,
// so concurrent requests cannot each see room under a limit.
func lockLoginCodes(ctx context.Context, tx *repo.Store, userID string) error {
	return tx.LockUser(ctx, repo.LockLoginCodes, userID)
}

// lockOpenAttempt locks the attempt and returns it when it can still log in,
// ErrNotFound otherwise.
func lockOpenAttempt(ctx context.Context, tx *repo.Store, attemptID string, now time.Time) (*core.LoginAttempt, error) {
	if _, err := uuid.Parse(attemptID); err != nil {
		return nil, service.ErrNotFound
	}

	a, err := notFound(tx.LockLoginAttempt(ctx, attemptID))
	if err != nil {
		return nil, err
	}

	if !open(a, now) {
		return nil, service.ErrNotFound
	}

	return a, nil
}

// StartLogin starts a login attempt for email and returns its id. The code
// is mailed only when the address belongs to a confirmed user who has been
// mailed fewer than maxCodesMailed codes lately and has not used up
// maxWrongTries; otherwise the attempt can never log in, and the caller
// cannot tell the difference.
func (s *Service) StartLogin(ctx context.Context, email, returnURL string) (string, error) {
	if len(s.codeKey) == 0 {
		return "", errNoCodeKey
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	now := s.clock()
	attempt := &core.LoginAttempt{
		ID:        id.String(),
		ReturnURL: returnURL,
		ExpiresAt: now.Add(CodeLifetime),
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		user, err := tx.UserByEmail(ctx, pgsession.NormalizeEmail(email), true)
		if errors.Is(err, repo.ErrNotFound) {
			return tx.InsertLoginAttempt(ctx, attempt)
		} else if err != nil {
			return err
		}

		attempt.UserID = null.StringFrom(user.ID)

		if err := lockLoginCodes(ctx, tx, user.ID); err != nil {
			return err
		}

		mailed, err := tx.LoginCodesSince(ctx, user.ID, now.Add(-codeLimitWindow))
		if err != nil {
			return err
		}

		wrongTries, err := tx.WrongLoginTriesSince(ctx, user.ID, now.Add(-wrongTryWindow))
		if err != nil {
			return err
		}

		if mailed >= maxCodesMailed || wrongTries >= maxWrongTries {
			return tx.InsertLoginAttempt(ctx, attempt)
		}

		code, err := newCode()
		if err != nil {
			return err
		}

		attempt.CodeHash = null.StringFrom(s.codeHash(attempt.ID, code))

		if err := tx.InsertLoginAttempt(ctx, attempt); err != nil {
			return err
		}

		return s.send(ctx, tx, mail.LoginCode(s.ident.From, attempt.ID, user.Email, code, CodeLifetime))
	})
	if err != nil {
		return "", err
	}

	return attempt.ID, nil
}

// startAttempt starts a login attempt for a user just created in tx and
// queues the mail build makes for its code, so the user, the attempt and the
// mail exist together or not at all. It does not look at the mail limit: the
// user is new.
func (s *Service) startAttempt(ctx context.Context, tx *repo.Store, user *core.User, build func(attemptID, code string) *mail.Envelope) (string, error) {
	if len(s.codeKey) == 0 {
		return "", errNoCodeKey
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	code, err := newCode()
	if err != nil {
		return "", err
	}

	now := s.clock()
	attempt := &core.LoginAttempt{
		ID:        id.String(),
		UserID:    null.StringFrom(user.ID),
		CodeHash:  null.StringFrom(s.codeHash(id.String(), code)),
		ExpiresAt: now.Add(CodeLifetime),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := tx.InsertLoginAttempt(ctx, attempt); err != nil {
		return "", err
	}

	return attempt.ID, s.send(ctx, tx, build(attempt.ID, code))
}

// FinishLogin checks code against the attempt and counts the try. A match
// uses the attempt up and returns its user and return URL. A wrong code is a
// ValidationError on "code", and so is any code for an attempt that has no
// user or whose user has had maxWrongTries lately, so neither can be told
// from a wrong guess; an attempt that is used, expired or out of tries is
// ErrNotFound. Finishing an attempt also confirms the user's email address
// when it is not confirmed yet: the code proves it.
func (s *Service) FinishLogin(ctx context.Context, attemptID, code string) (*core.User, string, error) {
	if len(s.codeKey) == 0 {
		return nil, "", errNoCodeKey
	}

	var (
		user      *core.User
		returnURL string
		wrong     bool
		confirmed bool
	)

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		now := s.clock()

		a, err := lockOpenAttempt(ctx, tx, attemptID, now)
		if err != nil {
			return err
		}

		a.Tries++
		cols := []string{core.LoginAttemptColumns.Tries}

		wrong = !a.UserID.Valid || !a.CodeHash.Valid
		if !wrong {
			// every try of the user waits here, so the count is exact
			if err := lockLoginCodes(ctx, tx, a.UserID.String); err != nil {
				return err
			}

			wrongTries, err := tx.WrongLoginTriesSince(ctx, a.UserID.String, now.Add(-wrongTryWindow))
			if err != nil {
				return err
			}

			wrong = wrongTries >= maxWrongTries || !hmac.Equal([]byte(a.CodeHash.String), []byte(s.codeHash(a.ID, code)))
		}

		if !wrong {
			a.UsedAt = null.TimeFrom(now)
			cols = append(cols, core.LoginAttemptColumns.UsedAt)
		}

		if err := tx.SaveLoginAttempt(ctx, a, cols...); err != nil {
			return err
		}

		if wrong {
			// commit the counted try; the caller gets the validation error
			return nil
		}

		returnURL = a.ReturnURL
		user, err = notFound(tx.UserByID(ctx, a.UserID.String))
		if err != nil || user.EmailConfirmedAt.Valid {
			return err
		}

		user.EmailConfirmedAt = null.TimeFrom(now)
		confirmed = true

		return tx.SaveUser(ctx, user)
	})
	if err != nil {
		return nil, "", err
	}

	if wrong {
		return nil, "", service.Invalid("code", "This code is not right. Check the mail and try again.")
	}

	if confirmed {
		s.notifyAdmin(ctx, "signup confirmed", admin.SignupConfirmed(s.ident.Site, s.ident.From, s.ident.AdminAddress, user))
	}

	return user, returnURL, nil
}

// IssueLoginCode replaces the code of an attempt that can still log in, gives
// it a fresh CodeLifetime and returns the code without mailing it, for the
// operator's command line and the test harness.
func (s *Service) IssueLoginCode(ctx context.Context, attemptID string) (string, error) {
	if len(s.codeKey) == 0 {
		return "", errNoCodeKey
	}

	var code string

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		now := s.clock()

		a, err := lockOpenAttempt(ctx, tx, attemptID, now)
		if err != nil {
			return err
		}

		if !a.UserID.Valid {
			return service.ErrNotFound
		}

		if code, err = newCode(); err != nil {
			return err
		}

		a.CodeHash = null.StringFrom(s.codeHash(a.ID, code))
		a.ExpiresAt = now.Add(CodeLifetime)

		return tx.SaveLoginAttempt(ctx, a, core.LoginAttemptColumns.CodeHash, core.LoginAttemptColumns.ExpiresAt)
	})
	if err != nil {
		return "", err
	}

	return code, nil
}

// LatestLoginAttempt returns the id of the newest attempt of the confirmed
// user with email that can still log in, or ErrNotFound.
func (s *Service) LatestLoginAttempt(ctx context.Context, email string) (string, error) {
	if len(s.codeKey) == 0 {
		return "", errNoCodeKey
	}

	user, err := notFound(s.store.UserByEmail(ctx, pgsession.NormalizeEmail(email), true))
	if err != nil {
		return "", err
	}

	a, err := notFound(s.store.NewestOpenLoginAttempt(ctx, user.ID, s.clock(), maxCodeTries))
	if err != nil {
		return "", err
	}

	return a.ID, nil
}
