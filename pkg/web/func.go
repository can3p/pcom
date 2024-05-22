package web

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/samber/lo"
	"github.com/samber/mo"
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

type MediationRequest struct {
	Requester *core.User
	Target    *core.User
	Request   *core.UserConnectionMediationRequest
}

type MediationResult struct {
	Mediation *core.UserConnectionMediator
	Mediator  *core.User
}

type ConnectionRequest struct {
	Requester  *core.User
	Request    *core.UserConnectionMediationRequest
	Mediations []*MediationResult
}

type Draft struct {
	PostID        string
	Subject       string
	LastUpdatedAt time.Time
}

type ControlsPage struct {
	*BasePage
	DirectConnections       core.UserSlice
	SecondDegreeConnections core.UserSlice
	WhitelistedConnections  core.UserSlice
	MediationRequests       []*MediationRequest
	ConnectionRequests      []*ConnectionRequest
	Drafts                  []*Draft
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

	connectionRequestsFromMediation, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(userID),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.Load(core.UserConnectionMediationRequestRels.WhoUser),
		qm.Load(qm.Rels(
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorRels.User,
		)),
		qm.Load(core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorWhere.Decision.EQ(core.ConnectionMediationDecisionSigned),
		),
	).All(ctx, db)

	if err != nil {
		panic(err)
	}

	connectionRequests := []*ConnectionRequest{}

	for _, req := range connectionRequestsFromMediation {
		connectionRequests = append(connectionRequests, &ConnectionRequest{
			Requester: req.R.WhoUser,
			Request:   req,
			Mediations: lo.Map(req.R.MediationUserConnectionMediators, func(m *core.UserConnectionMediator, idx int) *MediationResult {
				return &MediationResult{
					Mediator:  m.R.User,
					Mediation: m,
				}
			}),
		})
	}

	mediationRequestsDB, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.IN(directUserIDs),
		core.UserConnectionMediationRequestWhere.TargetUserID.IN(directUserIDs),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.Load(
			core.UserConnectionMediationRequestRels.WhoUser,
		),
		qm.Load(
			core.UserConnectionMediationRequestRels.TargetUser,
		),
		qm.Load(
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorWhere.UserID.EQ(userID),
		),
	).All(ctx, db)

	if err != nil {
		panic(err)
	}

	mediationRequestsDB = lo.Filter(mediationRequestsDB, func(req *core.UserConnectionMediationRequest, idx int) bool {
		return len(req.R.MediationUserConnectionMediators) == 0
	})

	mediationRequests := lo.Map(mediationRequestsDB, func(req *core.UserConnectionMediationRequest, idx int) *MediationRequest {
		return &MediationRequest{
			Requester: req.R.WhoUser,
			Target:    req.R.TargetUser,
			Request:   req,
		}
	})

	rawDrafts, err := core.Posts(
		core.PostWhere.UserID.EQ(userID),
		core.PostWhere.PublishedAt.IsNull(),
		qm.OrderBy("? DESC", core.PostColumns.UpdatedAt),
	).All(ctx, db)

	if err != nil {
		panic(err)
	}

	drafts := lo.Map(rawDrafts, func(d *core.Post, idx int) *Draft {
		return &Draft{
			PostID:        d.ID,
			Subject:       d.Subject,
			LastUpdatedAt: d.UpdatedAt.Time,
		}
	})

	controlsPage := &ControlsPage{
		BasePage:                getBasePage("Controls", userData),
		DirectConnections:       directUsers,
		SecondDegreeConnections: secondDegreeUsers,
		WhitelistedConnections:  whitelistedConnections,
		ConnectionRequests:      connectionRequests,
		MediationRequests:       mediationRequests,
		Drafts:                  drafts,
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
	Post     *postops.Post
	Comments []*postops.Comment
}

func SinglePost(c context.Context, db boil.ContextExecutor, userData *auth.UserData, postID string) mo.Result[*SinglePostPage] {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
	).One(c, db)

	if err == sql.ErrNoRows {
		return mo.Err[*SinglePostPage](ginhelpers.ErrNotFound)
	} else if err != nil {
		return mo.Err[*SinglePostPage](err)
	}

	author := post.R.User
	title := fmt.Sprintf("%s - %s", author.Username, post.Subject)

	connectionRadius, err := userops.GetConnectionRadius(c, db, userData.DBUser.ID, author.ID)

	if err != nil {
		return mo.Err[*SinglePostPage](err)
	}

	if connectionRadius.IsUnrelated() {
		return mo.Err[*SinglePostPage](ginhelpers.ErrForbidden)
	}

	singlePostPage := &SinglePostPage{
		BasePage: getBasePage(title, userData),
		Post:     postops.ConstructPost(userData.DBUser, post, connectionRadius),
	}

	if singlePostPage.Post.Capabilities.CanViewComments {
		rawComments, err := core.PostComments(
			core.PostCommentWhere.PostID.EQ(post.ID),
			qm.Load(core.PostCommentRels.User),
			qm.OrderBy("? ASC", core.PostCommentColumns.CreatedAt)).All(c, db)

		if err != nil {
			return mo.Err[*SinglePostPage](err)
		}

		singlePostPage.Comments = postops.ConstructComments(rawComments, connectionRadius)

	}

	return mo.Ok(singlePostPage)
}

type EditPostPage struct {
	*BasePage
	PostID        string
	Input         forms.PostFormInput
	LastUpdatedAt time.Time
	IsPublished   bool
}

func EditPost(c context.Context, db boil.ContextExecutor, userData *auth.UserData, postID string) mo.Result[*EditPostPage] {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
	).One(c, db)

	if err == sql.ErrNoRows {
		return mo.Err[*EditPostPage](ginhelpers.ErrNotFound)
	} else if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	author := post.R.User
	title := fmt.Sprintf("%s - %s", author.Username, post.Subject)

	connectionRadius, err := userops.GetConnectionRadius(c, db, userData.DBUser.ID, author.ID)

	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	capabilities := postops.GetPostCapabilities(userData.DBUser.ID, post.UserID, connectionRadius)

	if !capabilities.CanEdit {
		return mo.Err[*EditPostPage](ginhelpers.ErrForbidden)
	}

	editPostPage := &EditPostPage{
		BasePage: getBasePage(title, userData),
		PostID:   post.ID,
		Input: forms.PostFormInput{
			Subject:    post.Subject,
			Body:       post.Body,
			Visibility: post.VisibilityRadius,
		},
		LastUpdatedAt: post.UpdatedAt.Time,
		IsPublished:   post.PublishedAt.Valid,
	}

	return mo.Ok(editPostPage)
}

type UserHomePage struct {
	*BasePage
	Author            *core.User
	ConnectionRadius  userops.ConnectionRadius
	ConnectionAllowed bool
	MediationRequest  *core.UserConnectionMediationRequest
	Posts             []*postops.Post
}

func UserHome(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData, authorUsername string) mo.Result[*UserHomePage] {
	author, err := core.Users(
		core.UserWhere.Username.EQ(authorUsername),
	).One(ctx, db)

	if err == sql.ErrNoRows {
		return mo.Err[*UserHomePage](ginhelpers.ErrNotFound)
	} else if err != nil {
		return mo.Err[*UserHomePage](err)
	}

	title := fmt.Sprintf("%s - Journal", author.Username)

	connRadius, err := userops.GetConnectionRadius(ctx, db, userData.DBUser.ID, author.ID)

	if err != nil {
		return mo.Err[*UserHomePage](err)
	}

	var posts []*postops.Post

	if connRadius != userops.ConnectionRadiusUnknown {
		m := []qm.QueryMod{
			core.PostWhere.UserID.EQ(author.ID),
			core.PostWhere.PublishedAt.IsNotNull(),
			qm.Load(core.PostRels.User),
			qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
		}

		if connRadius == userops.ConnectionRadiusSecondDegree {
			m = append(m, core.PostWhere.VisibilityRadius.EQ(core.PostVisibilitySecondDegree))
		} else {
			m = append(m, qm.Load(core.PostRels.PostStat))
		}

		rawPosts, err := core.Posts(m...).All(ctx, db)

		if err != nil {
			return mo.Err[*UserHomePage](err)
		}

		posts = lo.Map(rawPosts, func(p *core.Post, idx int) *postops.Post {
			return postops.ConstructPost(userData.DBUser, p, connRadius)
		})
	}

	isConnectionAllowed, err := userops.IsConnectionAllowed(ctx, db, userData.DBUser.ID, author.ID)

	if err != nil {
		panic(err)
	}

	var mediationRequest *core.UserConnectionMediationRequest

	if connRadius == userops.ConnectionRadiusSecondDegree {
		mediationRequest, err = userops.GetMediationRequest(ctx, db, userData.DBUser.ID, author.ID)

		if err != nil {
			panic(err)
		}
	}

	userHomePage := &UserHomePage{
		BasePage:          getBasePage(title, userData),
		Author:            author,
		ConnectionRadius:  connRadius,
		ConnectionAllowed: isConnectionAllowed,
		MediationRequest:  mediationRequest,
		Posts:             posts,
	}

	return mo.Ok(userHomePage)
}

type FeedType int

const (
	FeedTypeDirect FeedType = iota
	FeedTypeExplore
)

type FeedPage struct {
	*BasePage
	Posts    []*postops.Post
	FeedType FeedType
}

func (fp *FeedPage) IsExplore() bool {
	return fp.FeedType == FeedTypeExplore
}

// TODO: allow the functions to return errors, since it will allow to use panic free methods and do better error handling
func DirectFeed(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*FeedPage] {
	user := userData.DBUser
	title := fmt.Sprintf("%s - Direct Feed", user.Username)

	directUserIDs, err := userops.GetDirectUserIDs(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	posts, err := core.Posts(
		core.PostWhere.UserID.IN(directUserIDs),
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	directFeedPage := &FeedPage{
		BasePage: getBasePage(title, userData),
		Posts: lo.Map(posts, func(p *core.Post, idx int) *postops.Post {
			return postops.ConstructPost(userData.DBUser, p, userops.ConnectionRadiusDirect)
		}),
		FeedType: FeedTypeDirect,
	}

	return mo.Ok(directFeedPage)
}

// TODO: allow the functions to return errors, since it will allow to use panic free methods and do better error handling
func ExploreFeed(ctx context.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*FeedPage] {
	user := userData.DBUser
	title := fmt.Sprintf("%s - Explore Feed", user.Username)

	_, secondDegreeUserIDs, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	posts, err := core.Posts(
		core.PostWhere.UserID.IN(secondDegreeUserIDs),
		core.PostWhere.PublishedAt.IsNotNull(),
		core.PostWhere.VisibilityRadius.EQ(core.PostVisibilitySecondDegree),
		qm.Load(core.PostRels.User),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	exploreFeedPage := &FeedPage{
		BasePage: getBasePage(title, userData),
		FeedType: FeedTypeExplore,
		Posts: lo.Map(posts, func(p *core.Post, idx int) *postops.Post {
			return postops.ConstructPost(userData.DBUser, p, userops.ConnectionRadiusSecondDegree)
		}),
	}

	return mo.Ok(exploreFeedPage)
}
