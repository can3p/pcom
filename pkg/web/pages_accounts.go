package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/gin-gonic/gin"
)

type SettingsPage struct {
	*BasePage
	AvailableInvites int64
	UsedInvites      []*model.UserInvitation
	ActiveAPIKey     *model.UserAPIKey
	FeedURL          string // private RSS feed URL, empty until a feed token exists
	GeneralSettings  *forms.SettingsGeneralForm
	UserStyles       *forms.SettingsUserStyles
	Profile          *forms.SettingsProfile
	Feeds            []*feeds.RssFeed
	FeedsSection     map[string]any // partial--settings_feeds.html
	InvitesSection   map[string]any // partial--settings_invites.html
}

func Settings(c *gin.Context, svc *accounts.Service, userData *auth.UserData, view *accounts.SettingsView) *SettingsPage {
	formUserStyles := forms.SettingsUserStylesNew(svc, userData.DBUser)
	formUserStyles.Input.Styles = view.UserStyles

	formProfile := forms.SettingsProfileNew(svc, userData.DBUser)
	formProfile.Input.About = view.ProfileAbout

	// the add feed form is only rendered here, so it needs no service
	feedsSection := forms.FeedsSection(view.Feeds, userData.DBUser, forms.NewAddFeedForm(nil, userData.DBUser).TemplateData())
	invitesSection := forms.InvitesSection(view.AvailableInvites, view.UsedInvites, forms.SendInviteFormNew(svc, userData.DBUser).TemplateData())

	return &SettingsPage{
		BasePage:         getBasePage(c, "Settings", userData),
		AvailableInvites: view.AvailableInvites,
		UsedInvites:      view.UsedInvites,
		ActiveAPIKey:     view.APIKey,
		FeedURL:          view.FeedURL,
		GeneralSettings:  forms.SettingsGeneralFormNew(svc, userData.DBUser),
		UserStyles:       formUserStyles,
		Profile:          formProfile,
		Feeds:            view.Feeds,
		FeedsSection:     feedsSection,
		InvitesSection:   invitesSection,
	}
}

type InvitePage struct {
	*BasePage
	Invite  *model.UserInvitation
	Inviter *model.User
}

// Invite is the page of an invitation, which has its inviter loaded.
func Invite(c *gin.Context, invite *model.UserInvitation, userData *auth.UserData) *InvitePage {
	return &InvitePage{
		BasePage: getBasePage(c, "Accept Invitation", userData),
		Invite:   invite,
		Inviter:  invite.User,
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
