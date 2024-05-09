package web

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/samber/lo"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
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

type ControlsPage struct {
	*BasePage
	DirectConnections       core.UserSlice
	SecondDegreeConnections core.UserSlice
	WhitelistedConnections  core.UserSlice
}

func Controls(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData) *ControlsPage {
	userID := userData.DBUser.ID
	directUserIDs, secondDegreeUserIDs, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, userID)

	// @TODO: all panics should be eliminated later
	if err != nil {
		panic(err)
	}

	directUsers := core.Users(core.UserWhere.ID.IN(directUserIDs)).AllP(ctx, db)
	secondDegreeUsers := core.Users(core.UserWhere.ID.IN(secondDegreeUserIDs)).AllP(ctx, db)

	whitelistedConnections := lo.Map(
		core.WhitelistedConnections(
			core.WhitelistedConnectionWhere.WhoID.EQ(userID),
			core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
			qm.Load(core.WhitelistedConnectionRels.AllowsWho),
		).AllP(ctx, db),
		func(conn *core.WhitelistedConnection, idx int) *core.User {
			return conn.R.AllowsWho
		})

	controlsPage := &ControlsPage{
		BasePage:                getBasePage("Controls", userData),
		DirectConnections:       directUsers,
		SecondDegreeConnections: secondDegreeUsers,
		WhitelistedConnections:  whitelistedConnections,
	}

	return controlsPage
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
	Author *core.User
	Post   *core.Post
}

func SinglePost(c context.Context, db boil.ContextExecutor, userData *auth.UserData, post *core.Post) *SinglePostPage {
	author := post.User().OneP(c, db)
	title := fmt.Sprintf("%s - %s", author.Username, post.Subject)

	singlePostPage := &SinglePostPage{
		BasePage: getBasePage(title, userData),
		Author:   author,
		Post:     post,
	}

	return singlePostPage
}

type UserHomePage struct {
	*BasePage
	Author           *core.User
	ConnectionRadius userops.ConnectionRadius
	Posts            core.PostSlice
}

// TODO: allow the functions to return errors, since it will allow to use panic free methods and do better error handling
func UserHome(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData, author *core.User) *UserHomePage {
	title := fmt.Sprintf("%s - Journal", author.Username)

	connRadius, err := userops.GetConnectionRadius(ctx, db, userData.DBUser.ID, author.ID)

	if err != nil {
		panic(err)
	}

	var posts core.PostSlice

	if connRadius != userops.ConnectionRadiusUnknown {
		m := []qm.QueryMod{
			core.PostWhere.UserID.EQ(author.ID),
			qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.CreatedAt)),
		}

		if connRadius == userops.ConnectionRadiusSecondDegree {
			m = append(m, core.PostWhere.VisbilityRadius.EQ(core.PostVisibilitySecondDegree))
		}

		posts, err = core.Posts(m...).All(ctx, db)

		if err != nil {
			panic(err)
		}
	}

	userHomePage := &UserHomePage{
		BasePage:         getBasePage(title, userData),
		Author:           author,
		ConnectionRadius: connRadius,
		Posts:            posts,
	}

	return userHomePage
}

type FeedType int

const (
	FeedTypeDirect FeedType = iota
	FeedTypeExplore
)

type FeedPage struct {
	*BasePage
	Posts    core.PostSlice
	FeedType FeedType
}

func (fp *FeedPage) IsExplore() bool {
	return fp.FeedType == FeedTypeExplore
}

// TODO: allow the functions to return errors, since it will allow to use panic free methods and do better error handling
func DirectFeed(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData) *FeedPage {
	user := userData.DBUser
	title := fmt.Sprintf("%s - Direct Feed", user.Username)

	directUserIDs, err := userops.GetDirectUserIDs(ctx, db, user.ID)

	if err != nil {
		panic(err)
	}

	directFeedPage := &FeedPage{
		BasePage: getBasePage(title, userData),
		Posts: core.Posts(
			core.PostWhere.UserID.IN(directUserIDs),
			qm.Load(core.PostRels.User),
			qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.ID)),
		).AllP(ctx, db),
		FeedType: FeedTypeDirect,
	}

	return directFeedPage
}

// TODO: allow the functions to return errors, since it will allow to use panic free methods and do better error handling
func ExploreFeed(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData) *FeedPage {
	user := userData.DBUser
	title := fmt.Sprintf("%s - Explore Feed", user.Username)

	_, secondDegreeUserIDs, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, user.ID)

	if err != nil {
		panic(err)
	}

	exploreFeedPage := &FeedPage{
		BasePage: getBasePage(title, userData),
		Posts: core.Posts(
			core.PostWhere.UserID.IN(secondDegreeUserIDs),
			core.PostWhere.VisbilityRadius.EQ(core.PostVisibilitySecondDegree),
			qm.Load(core.PostRels.User),
			qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.ID)),
		).AllP(ctx, db),
		FeedType: FeedTypeExplore,
	}

	return exploreFeedPage
}
