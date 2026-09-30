package web

import (
	"database/sql"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/gin-gonic/gin"
	"github.com/samber/mo"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SettingsPage struct {
	*BasePage
	AvailableInvites int64
	UsedInvites      core.UserInvitationSlice
	ActiveAPIKey     *core.UserAPIKey
	FeedURL          string // private RSS feed URL, empty until a feed token exists
	GeneralSettings  *forms.SettingsGeneralForm
	UserStyles       *forms.SettingsUserStyles
	Feeds            []*feedops.RssFeed
}

func Settings(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*SettingsPage] {
	totalInvites, err := core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(userData.DBUser.ID),
	).Count(c, db)

	if err != nil {
		return mo.Err[*SettingsPage](err)
	}

	usedInvites, err := core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(userData.DBUser.ID),
		core.UserInvitationWhere.InvitationEmail.IsNotNull(),
	).All(c, db)

	if err != nil {
		return mo.Err[*SettingsPage](err)
	}

	apiKey, err := core.UserAPIKeys(
		core.UserAPIKeyWhere.UserID.EQ(userData.DBUser.ID),
	).One(c, db)

	if err != nil && err != sql.ErrNoRows {
		return mo.Err[*SettingsPage](err)
	}

	feedToken, err := repo.FeedTokenForUser(c, db, userData.DBUser.ID)

	if err != nil {
		return mo.Err[*SettingsPage](err)
	}

	feedURL := ""
	if feedToken != nil {
		feedURL = links.AbsLink("private_user_feed", feedToken.Token)
	}

	formUserStyles := forms.SettingsUserStylesNew(userData.DBUser)

	userStyles, err := core.UserStyles(
		core.UserStyleWhere.UserID.EQ(userData.DBUser.ID),
	).One(c, db)

	if err != nil && err != sql.ErrNoRows {
		return mo.Err[*SettingsPage](err)
	} else if userStyles != nil {
		formUserStyles.Input.Styles = userStyles.Styles
	}

	feeds, err := feedops.GetRssFeeds(c, db, userData.DBUser.ID)

	if err != nil {
		return mo.Err[*SettingsPage](err)
	}

	settingsPage := &SettingsPage{
		BasePage:         getBasePage(c, "Settings", userData),
		AvailableInvites: totalInvites - int64(len(usedInvites)),
		UsedInvites:      usedInvites,
		ActiveAPIKey:     apiKey,
		FeedURL:          feedURL,
		GeneralSettings:  forms.SettingsGeneralFormNew(userData.DBUser),
		UserStyles:       formUserStyles,
		Feeds:            feeds,
	}

	return mo.Ok(settingsPage)
}

type InvitePage struct {
	*BasePage
	Invite  *core.UserInvitation
	Inviter *core.User
}

func Invite(c *gin.Context, db boil.ContextExecutor, invite *core.UserInvitation, userData *auth.UserData) *InvitePage {
	invitePage := &InvitePage{
		BasePage: getBasePage(c, "Accept Invitation", userData),
		Invite:   invite,
		Inviter:  invite.User().OneP(c, db),
	}

	return invitePage
}

type LoginPage struct {
	*BasePage
	ReturnURL string
	Sign      string
}

func Login(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData, returnUrl string, sign string) *LoginPage {
	invitePage := &LoginPage{
		BasePage:  getBasePage(c, "Login", userData),
		ReturnURL: returnUrl,
		Sign:      sign,
	}

	return invitePage
}
