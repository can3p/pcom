package web

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
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
	Author *core.User
	Posts  core.PostSlice
}

// TODO: allow the functions to return errors, since it will allow to use panic free methods and do better error handling
func UserHome(c context.Context, db boil.ContextExecutor, userData *auth.UserData, author *core.User) *UserHomePage {
	title := fmt.Sprintf("%s - Journal", author.Username)

	userHomePage := &UserHomePage{
		BasePage: getBasePage(title, userData),
		Author:   author,
		Posts: core.Posts(
			core.PostWhere.UserID.EQ(author.ID),
			qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.CreatedAt)),
		).AllP(c, db),
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

	connections := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(user.ID),
	).AllP(ctx, db)

	userIDs := []string{}

	for _, conn := range connections {
		userIDs = append(userIDs, conn.User2ID)
	}

	directFeedPage := &FeedPage{
		BasePage: getBasePage(title, userData),
		Posts: core.Posts(
			core.PostWhere.UserID.IN(userIDs),
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

	connections := core.UserConnections(
		core.UserConnectionWhere.User1ID.EQ(user.ID),
	).AllP(ctx, db)

	// we want to explude direct connection as well as the user themselves from indirect connections results
	// this has to be done, since the connection graph is undirected
	directUserIDs := lo.Map(connections, func(conn *core.UserConnection, index int) string { return conn.User2ID })
	directUserIDs = append(directUserIDs, user.ID)

	type userID struct {
		UserID string `boil:"user_id"`
	}

	var indirectUserIDStruct []*userID

	// https://www.linkedin.com/pulse/you-dont-need-graph-database-modeling-graphs-trees-viktor-qvarfordt-efzof/
	core.NewQuery(
		qm.Select("conn2.user2_id as user_id"),
		qm.From("user_connections as conn1"),
		qm.LeftOuterJoin("user_connections as conn2 on conn1.user2_id = conn2.user1_id"),
		qm.Where("conn1.user1_id = ?", user.ID),
	).BindP(ctx, db, &indirectUserIDStruct)

	indirectUserIDs := lo.Without(
		lo.Map(indirectUserIDStruct, func(conn *userID, index int) string { return conn.UserID }),
		directUserIDs...,
	)

	exploreFeedPage := &FeedPage{
		BasePage: getBasePage(title, userData),
		Posts: core.Posts(
			core.PostWhere.UserID.IN(indirectUserIDs),
			core.PostWhere.VisbilityRadius.EQ(core.PostVisibilitySecondDegree),
			qm.Load(core.PostRels.User),
			qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.ID)),
		).AllP(ctx, db),
	}

	return exploreFeedPage
}
