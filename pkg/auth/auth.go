package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

const (
	userkey = "user"
)

func Auth(c *gin.Context, db *sqlx.DB) {
	session := sessions.Default(c)
	user := session.Get(userkey)

	if user == nil {
		c.Next()

		return
	}

	if err := pgsession.SetUser(c, db, user.(string)); err != nil {
		log.Printf("Failed to save user to pgsession, auth won't work as expected: %s", err)
	}

	c.Next()
}

func AuthAPI(c *gin.Context, db *sqlx.DB) {
	apiToken := c.GetHeader("Authorization")

	parts := strings.Split(apiToken, " ")

	if apiToken == "" || parts[0] != "Bearer" || len(parts) != 2 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	userToken, err := core.UserAPIKeys(
		core.UserAPIKeyWhere.APIKey.EQ(parts[1]),
	).One(c, db)

	if err == sql.ErrNoRows {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		slog.Warn("Failed to fetch user token", "err", err)
		return
	}

	if err := pgsession.SetUser(c, db, userToken.UserID); err != nil {
		log.Printf("Failed to save user to pgsession, auth won't work as expected: %s", err)
	}

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

func CheckCredentials(c *gin.Context, db boil.ContextExecutor, email string, password string) error {
	h := pgsession.HashUserPwd(email, password)

	_, err := core.Users(
		core.UserWhere.Email.EQ(email),
		core.UserWhere.Pwdhash.EQ(null.StringFrom(h)),
		core.UserWhere.EmailConfirmedAt.IsNotNull(),
	).One(c.Request.Context(), db)

	if err != nil {
		if err == sql.ErrNoRows {
			return errors.Errorf("Bad credentials")
		}

		return err
	}

	return nil
}

func Login(c *gin.Context, db boil.ContextExecutor, email string, password string) error {
	session := sessions.Default(c)
	h := pgsession.HashUserPwd(email, password)

	user, err := core.Users(
		core.UserWhere.Email.EQ(email),
		core.UserWhere.Pwdhash.EQ(null.StringFrom(h)),
		core.UserWhere.EmailConfirmedAt.IsNotNull(),
	).One(c.Request.Context(), db)

	if err != nil {
		if err == sql.ErrNoRows {
			return errors.Errorf("Bad credentials")
		}

		panic(err)
	}

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
	session.Delete(userkey)
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

	storedToken := session.Get("csrf_token")

	if storedToken == nil {
		storedToken = uuid.NewString()
		session.Set("csrf_token", storedToken)

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

func AddFlash(c *gin.Context, flash interface{}, vars ...string) {
	session := sessions.Default(c)

	session.AddFlash(flash, vars...)
	if err := session.Save(); err != nil {
		log.Printf("Failed to save session: %v", err)
	}
}

func GetFlashes(c *gin.Context, vars ...string) []interface{} {
	session := sessions.Default(c)

	flashes := session.Flashes(vars...)

	if len(flashes) != 0 {
		if err := session.Save(); err != nil {
			log.Printf("error in flashes saving session: %v", err)
		}
	}

	return flashes
}

func AcceptInvite(ctx context.Context, db boil.ContextExecutor, s sender.Sender, invite *core.UserInvitation, username string, password string) error {
	if password == "" || username == "" {
		return errors.Errorf("Not enough data")
	}

	email := invite.InvitationEmail.String

	u := &core.User{
		ID:                uuid.NewString(),
		Email:             email,
		Username:          username,
		Pwdhash:           null.StringFrom(pgsession.HashUserPwd(email, password)),
		EmailConfirmedAt:  null.TimeFrom(time.Now()),
		SignupAttribution: null.StringFrom("accepted_invite"),
	}

	if err := u.Insert(ctx, db, boil.Infer()); err != nil {
		return err
	}

	invite.CreatedUserID = null.StringFrom(u.ID)

	if _, err := invite.Update(ctx, db, boil.Infer()); err != nil {
		return err
	}

	// give every new user one new invite to make things (slowly) spread
	newInvite := &core.UserInvitation{
		ID:     uuid.NewString(),
		UserID: u.ID,
	}

	if _, _, err := userops.CreateConnection(ctx, db, invite.UserID, u.ID); err != nil {
		return err
	}

	admin.NotifyNewUser(ctx, db, s, u)

	return newInvite.Insert(ctx, db, boil.Infer())
}

// Signup assumes the transaction is already began
func Signup(ctx context.Context, db boil.ContextExecutor, sender sender.Sender, email string, username, password string, attribution string) (*core.User, error) {
	if password == "" || email == "" || username == "" {
		return nil, errors.Errorf("Not enough data")
	}

	u := &core.User{
		ID:                uuid.NewString(),
		Email:             email,
		Username:          username,
		Pwdhash:           null.StringFrom(pgsession.HashUserPwd(email, password)),
		EmailConfirmSeed:  null.StringFrom(uuid.NewString()),
		SignupAttribution: null.NewString(attribution, attribution != ""),
	}

	if err := u.Insert(ctx, db, boil.Infer()); err != nil {
		return nil, err
	}

	admin.NotifyNewUser(ctx, db, sender, u)

	return u, nil
}
