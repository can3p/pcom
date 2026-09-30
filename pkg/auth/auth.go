package auth

import (
	"crypto/sha256"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	userkey      = "user"
	csrfTokenKey = "csrf_token"
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

func EnforceAuth(c *gin.Context) {
	userData := GetUserData(c)

	if !userData.IsLoggedIn {
		RedirectToLogin(c)
		c.Abort()
		return
	}

	c.Next()
}

func HashValue(v string) string {
	sessionSalt := os.Getenv("SESSION_SALT")
	data := []byte(sessionSalt + ":" + v)
	hash := sha256.Sum256(data)

	return fmt.Sprintf("%x", hash)
}

func RedirectToLogin(c *gin.Context) {
	path := c.Request.URL.Path
	// we need to sign return url
	c.Redirect(http.StatusFound, links.Link("login", "return_url", path, "sign", HashValue(path)))
}

func EnforceReferer(c *gin.Context) {
	referer := c.Request.Header.Get("referer")
	if referer == "" || !strings.HasPrefix(referer, util.SiteRoot()) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	c.Next()
}

// Login checks the credentials and starts the session of the user they
// belong to.
func Login(c *gin.Context, accounts *accounts.Service, email string, password string) error {
	user, err := accounts.Authenticate(c.Request.Context(), email, password)
	if err != nil {
		return err
	}

	return StartSession(c, user)
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
