package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
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

// codeDigits is the length of a code, part of its format (the mail, the
// form): 10^8 of them leave a guesser who gets WrongTries a WrongTriesWindow
// nothing to hope for.
const codeDigits = 8

// LoginLimits are the limits of login by code. pkg/config sets them
// (LOGIN_* settings) with DefaultLoginLimits as the defaults.
type LoginLimits struct {
	// CodeLifetime is how long a login code works.
	CodeLifetime time.Duration
	// CodeTries is how many wrong codes an attempt allows; after that it is
	// dead.
	CodeTries int
	// CodesMailed codes are mailed to one user per CodesMailedWindow.
	CodesMailed       int
	CodesMailedWindow time.Duration
	// After WrongTries wrong codes on a user's attempts started within
	// WrongTriesWindow, no code logs the user in and none is mailed until the
	// window moves past them.
	WrongTries       int
	WrongTriesWindow time.Duration
	// UnconfirmedLifetime is how long an account whose owner never typed a
	// code is kept; after it the address and the username are free again.
	UnconfirmedLifetime time.Duration
	// PruneEvery is how often RunPruner deletes dead attempts and accounts.
	PruneEvery time.Duration
}

// DefaultLoginLimits are the limits a service built without WithLoginLimits
// uses, and the configuration's defaults. Read it; never change it.
var DefaultLoginLimits = LoginLimits{
	CodeLifetime:        15 * time.Minute,
	CodeTries:           5,
	CodesMailed:         3,
	CodesMailedWindow:   15 * time.Minute,
	WrongTries:          10,
	WrongTriesWindow:    time.Hour,
	UnconfirmedLifetime: 24 * time.Hour,
	PruneEvery:          time.Hour,
}

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
func (s *Service) open(a *core.LoginAttempt, now time.Time) bool {
	return !a.UsedAt.Valid && a.ExpiresAt.After(now) && a.WrongTries < s.login.CodeTries
}

// lockLoginCodes serializes everything that counts a user's codes and tries,
// so concurrent requests cannot each see room under a limit.
func lockLoginCodes(ctx context.Context, tx *repo.Store, userID string) error {
	return tx.LockUser(ctx, repo.LockLoginCodes, userID)
}

// lockOpenAttempt locks the attempt and returns it when it can still log in,
// ErrNotFound otherwise.
func (s *Service) lockOpenAttempt(ctx context.Context, tx *repo.Store, attemptID string, now time.Time) (*core.LoginAttempt, error) {
	if _, err := uuid.Parse(attemptID); err != nil {
		return nil, service.ErrNotFound
	}

	a, err := notFound(tx.LockLoginAttempt(ctx, attemptID))
	if err != nil {
		return nil, err
	}

	if !s.open(a, now) {
		return nil, service.ErrNotFound
	}

	return a, nil
}

// StartLogin starts a login attempt for email and returns its id. The code
// is mailed only when the address belongs to a user who has been mailed
// fewer than CodesMailed codes lately and has not used up WrongTries;
// otherwise the attempt can never log in, and the caller cannot tell the
// difference. The user need not be confirmed: a signup whose code expired
// logs in here, and the code confirms the address.
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
		ExpiresAt: now.Add(s.login.CodeLifetime),
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		user, err := tx.UserByEmail(ctx, pgsession.NormalizeEmail(email))
		if errors.Is(err, repo.ErrNotFound) {
			return tx.InsertLoginAttempt(ctx, attempt)
		} else if err != nil {
			return err
		}

		attempt.UserID = null.StringFrom(user.ID)

		if err := lockLoginCodes(ctx, tx, user.ID); err != nil {
			return err
		}

		mailed, err := tx.LoginCodesSince(ctx, user.ID, now.Add(-s.login.CodesMailedWindow))
		if err != nil {
			return err
		}

		wrongTries, err := tx.WrongLoginTriesSince(ctx, user.ID, now.Add(-s.login.WrongTriesWindow))
		if err != nil {
			return err
		}

		if mailed >= int64(s.login.CodesMailed) || wrongTries >= int64(s.login.WrongTries) {
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

		return s.send(ctx, tx, mail.LoginCode(s.ident.From, attempt.ID, user.Email, code, s.login.CodeLifetime))
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
		ExpiresAt: now.Add(s.login.CodeLifetime),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := tx.InsertLoginAttempt(ctx, attempt); err != nil {
		return "", err
	}

	return attempt.ID, s.send(ctx, tx, build(attempt.ID, code))
}

// FinishLogin checks code against the attempt and counts a wrong one. A match
// uses the attempt up and returns its user and return URL. A wrong code is a
// ValidationError on "code", and so is any code for an attempt that has no
// user or whose user has had WrongTries lately, so neither can be told
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

		a, err := s.lockOpenAttempt(ctx, tx, attemptID, now)
		if err != nil {
			return err
		}

		wrong = !a.UserID.Valid || !a.CodeHash.Valid
		if !wrong {
			// every try of the user waits here, so the count is exact
			if err := lockLoginCodes(ctx, tx, a.UserID.String); err != nil {
				return err
			}

			wrongTries, err := tx.WrongLoginTriesSince(ctx, a.UserID.String, now.Add(-s.login.WrongTriesWindow))
			if err != nil {
				return err
			}

			wrong = wrongTries >= int64(s.login.WrongTries) || !hmac.Equal([]byte(a.CodeHash.String), []byte(s.codeHash(a.ID, code)))
		}

		col := core.LoginAttemptColumns.UsedAt
		if wrong {
			a.WrongTries++
			col = core.LoginAttemptColumns.WrongTries
		} else {
			a.UsedAt = null.TimeFrom(now)
		}

		if err := tx.SaveLoginAttempt(ctx, a, col); err != nil {
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

		a, err := s.lockOpenAttempt(ctx, tx, attemptID, now)
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
		a.ExpiresAt = now.Add(s.login.CodeLifetime)

		return tx.SaveLoginAttempt(ctx, a, core.LoginAttemptColumns.CodeHash, core.LoginAttemptColumns.ExpiresAt)
	})
	if err != nil {
		return "", err
	}

	return code, nil
}

// LatestLoginAttempt returns the id of the newest attempt of the user with
// email that can still log in, or ErrNotFound.
func (s *Service) LatestLoginAttempt(ctx context.Context, email string) (string, error) {
	if len(s.codeKey) == 0 {
		return "", errNoCodeKey
	}

	user, err := notFound(s.store.UserByEmail(ctx, pgsession.NormalizeEmail(email)))
	if err != nil {
		return "", err
	}

	now := s.clock()

	attempts, err := s.store.UnexpiredLoginAttempts(ctx, user.ID, now)
	if err != nil {
		return "", err
	}

	for _, a := range attempts {
		if s.open(a, now) {
			return a.ID, nil
		}
	}

	return "", service.ErrNotFound
}

// PruneLoginAttempts deletes the attempts no limit looks at any more: those
// that expired more than WrongTriesWindow ago, and so were created even earlier.
// It returns how many it deleted.
func (s *Service) PruneLoginAttempts(ctx context.Context) (int64, error) {
	return s.store.DeleteLoginAttemptsExpiredBefore(ctx, s.clock().Add(-s.login.WrongTriesWindow))
}

// PruneUnconfirmedUsers deletes the accounts created more than
// UnconfirmedLifetime ago whose owner never typed a code. An account an
// operator gave invitations to is kept. It deletes one at a time, so a delete
// that fails anyway is logged and does not hold up the rest. It returns how
// many it deleted.
func (s *Service) PruneUnconfirmedUsers(ctx context.Context) (int, error) {
	ids, err := s.store.UnconfirmedUserIDsCreatedBefore(ctx, s.clock().Add(-s.login.UnconfirmedLifetime))
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, id := range ids {
		ok, err := s.store.DeleteUnconfirmedUser(ctx, id)
		if err != nil {
			slog.Warn("Failed to delete an unconfirmed user", "user_id", id, "err", err.Error())
			continue
		}

		if ok {
			deleted++
		}
	}

	return deleted, nil
}

// RunPruner prunes login attempts and unconfirmed accounts every PruneEvery
// until ctx is done.
func (s *Service) RunPruner(ctx context.Context) {
	ticker := time.NewTicker(s.login.PruneEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := s.PruneLoginAttempts(ctx); err != nil {
				slog.Warn("Failed to prune login attempts", "err", err.Error())
			}

			if _, err := s.PruneUnconfirmedUsers(ctx); err != nil {
				slog.Warn("Failed to prune unconfirmed users", "err", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}
