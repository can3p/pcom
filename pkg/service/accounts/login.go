package accounts

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/volatiletech/null/v8"
)

func badCredentials() error {
	return service.Invalid("", "Bad credentials")
}

// find returns the confirmed user that email and password log in as, and
// whether their stored password hash is a legacy one to replace.
func (s *Service) find(ctx context.Context, email, password string) (*core.User, bool, error) {
	user, err := s.store.UserByEmail(ctx, pgsession.NormalizeEmail(email), true)
	if err == repo.ErrNotFound {
		return nil, false, badCredentials()
	} else if err != nil {
		return nil, false, err
	}

	if !user.Pwdhash.Valid {
		return nil, false, badCredentials()
	}

	ok, needsRehash := pgsession.CheckUserPwd(user.Pwdhash.String, user.Email, password)
	if !ok {
		return nil, false, badCredentials()
	}

	return user, needsRehash, nil
}

// CheckCredentials reports whether email and password belong to a confirmed
// account, with a ValidationError when they do not.
func (s *Service) CheckCredentials(ctx context.Context, email, password string) error {
	_, _, err := s.find(ctx, email, password)

	return err
}

// Authenticate returns the confirmed user that email and password log in as.
// A password stored with the legacy hash is stored again with the current
// one.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*core.User, error) {
	user, needsRehash, err := s.find(ctx, email, password)
	if err != nil {
		return nil, err
	}

	if needsRehash {
		user.Pwdhash = null.StringFrom(pgsession.HashPassword(password))

		if err := s.store.SaveUser(ctx, user, core.UserColumns.Pwdhash); err != nil {
			return nil, fmt.Errorf("failed to rehash the password: %w", err)
		}
	}

	return user, nil
}

// ChangePassword replaces the actor's password after checking the old one.
func (s *Service) ChangePassword(ctx context.Context, actor *core.User, oldPassword, newPassword string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if ok, _ := pgsession.CheckUserPwd(actor.Pwdhash.String, actor.Email, oldPassword); !ok {
		return service.Invalid("old_password", "old password is not correct")
	}

	actor.Pwdhash = null.StringFrom(pgsession.HashPassword(newPassword))

	if err := s.store.SaveUser(ctx, actor); err != nil {
		return fmt.Errorf("failed to save to the db: %w", err)
	}

	return nil
}
