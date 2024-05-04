package web

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type BasePage struct {
	ProjectName string
	Name        string
	User        *auth.UserData
}

func getBasePage(name string, userData *auth.UserData) *BasePage {
	return &BasePage{
		Name:        name,
		User:        userData,
		ProjectName: "pcom",
	}
}

func Index(c context.Context, db boil.ContextExecutor, userData *auth.UserData) *BasePage {
	return getBasePage("Super cool pcom", userData)
}

func Controls(c context.Context, db boil.ContextExecutor, userData *auth.UserData) *BasePage {
	return getBasePage("Controls", userData)
}

func Write(c context.Context, db boil.ContextExecutor, userData *auth.UserData) *BasePage {
	return getBasePage("New Post", userData)
}

type SettingsPage struct {
	*BasePage
	AvailableInvites int64
	UsedInvites      core.UserInvitationSlice
}

func Settings(c context.Context, db boil.ContextExecutor, userData *auth.UserData) *SettingsPage {
	totalInvites := core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(userData.DBUser.ID),
	).CountP(c, db)

	usedInvites := core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(userData.DBUser.ID),
		core.UserInvitationWhere.InvitationEmail.IsNotNull(),
	).AllP(c, db)

	settingsPage := &SettingsPage{
		BasePage:         getBasePage("Settings", userData),
		AvailableInvites: totalInvites - int64(len(usedInvites)),
		UsedInvites:      usedInvites,
	}

	return settingsPage
}

type InvitePage struct {
	*BasePage
	Invite  *core.UserInvitation
	Inviter *core.User
}

func Invite(c context.Context, db boil.ContextExecutor, invite *core.UserInvitation, userData *auth.UserData) *InvitePage {
	invitePage := &InvitePage{
		BasePage: getBasePage("Accept Invitation", userData),
		Invite:   invite,
		Inviter:  invite.User().OneP(c, db),
	}

	return invitePage
}

type SinglePostPage struct {
	*BasePage
	Post *core.Post
}

func SinglePost(c context.Context, db boil.ContextExecutor, userData *auth.UserData, post *core.Post) *SinglePostPage {
	author := post.User().OneP(c, db)
	title := fmt.Sprintf("%s - %s", author.Username, post.Subject)

	singlePostPage := &SinglePostPage{
		BasePage: getBasePage(title, userData),
		Post:     post,
	}

	return singlePostPage
}
