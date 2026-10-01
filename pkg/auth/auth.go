package auth

import (
	"crypto/sha256"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/can3p/gogo/apperr"
	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	userkey      = "user"
	csrfTokenKey = "csrf_token"
	// loginAttemptKey holds the id of the login attempt the visitor is
	// entering a code for, so the code form never carries it.
	loginAttemptKey = "login_attempt"
)

func setUser(c *gin.Context, u *core.User) {
	pgsession.SetLoadedUser(c, u)
}

func Auth(c *gin.Context, accounts *accounts.Service) {
	session := sessions.Default(c)
	user := session.Get(userkey)

	if user == nil {
		c.Next()

		return
	}

	if u, err := accounts.UserByID(c.Request.Context(), user.(string)); err != nil {
		log.Printf("Failed to save user to pgsession, auth won't work as expected: %s", err)
	} else {
		setUser(c, u)
	}

	c.Next()
}

func AuthAPI(c *gin.Context, accounts *accounts.Service) {
	apiToken := c.GetHeader("Authorization")

	parts := strings.Split(apiToken, " ")

	if apiToken == "" || parts[0] != "Bearer" || len(parts) != 2 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	user, err := accounts.UserByAPIKey(c.Request.Context(), parts[1])

	if errors.Is(err, service.ErrNotFound) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		slog.Warn("Failed to fetch user token", "err", err)
		return
	}

	setUser(c, user)

	c.Next()
}

// EnforceAuth sends a visitor who is not logged in to the login page through
// the redirect the router configures with ginhelpers.Options.RedirectToLogin.
func EnforceAuth(c *gin.Context) {
	userData := GetUserData(c)

	if !userData.IsLoggedIn {
		ginhelpers.HTMLError(c, apperr.ErrNeedsLogin)
		c.Abort()
		return
	}

	c.Next()
}

// HashValue signs v with the session salt.
func HashValue(salt, v string) string {
	data := []byte(salt + ":" + v)
	hash := sha256.Sum256(data)

	return fmt.Sprintf("%x", hash)
}

// RedirectToLogin answers with a redirect to the login page, which carries
// the current path and its signature.
func RedirectToLogin(salt string) func(*gin.Context) {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// we need to sign return url
		c.Redirect(http.StatusFound, links.Link("login", "return_url", path, "sign", HashValue(salt, path)))
	}
}

// EnforceReferer only lets through requests that come from a page of the site.
func EnforceReferer(siteRoot string) gin.HandlerFunc {
	return func(c *gin.Context) {
		referer := c.Request.Header.Get("referer")
		if referer == "" || !strings.HasPrefix(referer, siteRoot) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Next()
	}
}

// SetLoginAttempt remembers the login attempt of the visitor in the session.
func SetLoginAttempt(c *gin.Context, attemptID string) error {
	session := sessions.Default(c)
	session.Set(loginAttemptKey, attemptID)

	return errors.Wrap(session.Save(), "Failed to save session")
}

// LoginAttempt returns the id of the visitor's login attempt, empty when there
// is none.
func LoginAttempt(c *gin.Context) string {
	id, _ := sessions.Default(c).Get(loginAttemptKey).(string)

	return id
}

// StartSession logs user in: the session gets a new ID, a CSRF token and the
// user.
func StartSession(c *gin.Context, user *core.User) error {
	session := sessions.Default(c)

	if err := pgsession.Regenerate(session); err != nil {
		return err
	}

	session.Set(csrfTokenKey, uuid.NewString())
	session.Set(userkey, user.ID)

	if err := session.Save(); err != nil {
		return errors.Wrapf(err, "Failed to save session")
	}

	return nil
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	user := session.Get(userkey)
	c.Header("HX-Redirect", "/")
	c.Status(http.StatusOK)

	if user == nil {
		return
	}

	if err := pgsession.Regenerate(session); err != nil {
		slog.Warn("Failed to regenerate session on logout", "err", err)
		session.Delete(userkey)
	}

	if err := session.Save(); err != nil {
		return
	}
	c.Abort()
}

type UserData struct {
	User       *pgsession.User
	DBUser     *core.User
	IsLoggedIn bool
	CSRFToken  string
}

func GetUserData(c *gin.Context) UserData {
	var out UserData

	u := pgsession.GetUser(c)

	session := sessions.Default(c)

	storedToken := session.Get(csrfTokenKey)

	if storedToken == nil {
		storedToken = uuid.NewString()
		session.Set(csrfTokenKey, storedToken)

		if err := session.Save(); err != nil {
			slog.Warn(errors.Wrapf(err, "Failed to save session").Error())
		}
	}

	out.CSRFToken = storedToken.(string)
	out.IsLoggedIn = u != nil
	out.User = u
	if u != nil {
		out.DBUser = u.DBUser
	}

	return out
}

// this is a lame way of doing the auth. Ideally
// the controller code should simply read the user from the
// context and that's it
func GetAPIUserData(c *gin.Context) UserData {
	var out UserData

	u := pgsession.GetUser(c)

	out.IsLoggedIn = u != nil
	out.User = u
	if u != nil {
		out.DBUser = u.DBUser
	}

	return out
}

func AddFlash(c *gin.Context, flash any, vars ...string) {
	session := sessions.Default(c)

	session.AddFlash(flash, vars...)
	if err := session.Save(); err != nil {
		log.Printf("Failed to save session: %v", err)
	}
}

func GetFlashes(c *gin.Context, vars ...string) []any {
	session := sessions.Default(c)

	flashes := session.Flashes(vars...)

	if len(flashes) != 0 {
		if err := session.Save(); err != nil {
			log.Printf("error in flashes saving session: %v", err)
		}
	}

	return flashes
}
