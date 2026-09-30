package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/gin-gonic/gin"
)

type SettingsPage struct {
	*BasePage
	AvailableInvites int64
	UsedInvites      core.UserInvitationSlice
	ActiveAPIKey     *core.UserAPIKey
	FeedURL          string // private RSS feed URL, empty until a feed token exists
	GeneralSettings  *forms.SettingsGeneralForm
	UserStyles       *forms.SettingsUserStyles
	Feeds            []*feeds.RssFeed
}

func Settings(c *gin.Context, svc *accounts.Service, userData *auth.UserData, view *accounts.SettingsView) *SettingsPage {
	formUserStyles := forms.SettingsUserStylesNew(svc, userData.DBUser)
	formUserStyles.Input.Styles = view.UserStyles

	return &SettingsPage{
		BasePage:         getBasePage(c, "Settings", userData),
		AvailableInvites: view.AvailableInvites,
		UsedInvites:      view.UsedInvites,
		ActiveAPIKey:     view.APIKey,
		FeedURL:          view.FeedURL,
		GeneralSettings:  forms.SettingsGeneralFormNew(svc, userData.DBUser),
		UserStyles:       formUserStyles,
		Feeds:            view.Feeds,
	}
}

type InvitePage struct {
	*BasePage
	Invite  *core.UserInvitation
	Inviter *core.User
}

// Invite is the page of an invitation, which has its inviter loaded.
func Invite(c *gin.Context, invite *core.UserInvitation, userData *auth.UserData) *InvitePage {
	return &InvitePage{
		BasePage: getBasePage(c, "Accept Invitation", userData),
		Invite:   invite,
		Inviter:  invite.R.User,
	}
}

type LoginPage struct {
	*BasePage
	ReturnURL string
	Sign      string
}

func Login(c *gin.Context, userData *auth.UserData, returnUrl string, sign string) *LoginPage {
	return &LoginPage{
		BasePage:  getBasePage(c, "Login", userData),
		ReturnURL: returnUrl,
		Sign:      sign,
	}
}
