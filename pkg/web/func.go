package web

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

type BasePage struct {
	ProjectName string
	Name        string
	User        *auth.UserData
	StyleNonce  *string
	ScriptNonce *string
}

func getBasePage(c *gin.Context, name string, userData *auth.UserData) *BasePage {
	return &BasePage{
		Name:        name,
		User:        userData,
		ProjectName: "pcom",
		StyleNonce:  csp.GetStyleNonce(c),
		ScriptNonce: csp.GetScriptNonce(c),
	}
}

func Index(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData) *BasePage {
	return getBasePage(c, "Social network for private groups", userData)
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

func Controls(ctx *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*ControlsPage] {
	userID := userData.DBUser.ID
	directUserIDs, secondDegreeUserIDs, _, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, userID)

	if err != nil {
		return mo.Err[*ControlsPage](err)
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
		return mo.Err[*ControlsPage](err)
	}

	connectionRequests := []*ConnectionRequest{}

	for _, req := range connectionRequestsFromMediation {
		if len(req.R.MediationUserConnectionMediators) == 0 {
			continue
		}

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
		return mo.Err[*ControlsPage](err)
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
		return mo.Err[*ControlsPage](err)
	}

	drafts := lo.Map(rawDrafts, func(d *core.Post, idx int) *Draft {
		return &Draft{
			PostID:        d.ID,
			Subject:       d.Subject,
			LastUpdatedAt: d.UpdatedAt.Time,
		}
	})

	controlsPage := &ControlsPage{
		BasePage:                getBasePage(ctx, "Controls", userData),
		DirectConnections:       directUsers,
		SecondDegreeConnections: secondDegreeUsers,
		WhitelistedConnections:  whitelistedConnections,
		ConnectionRequests:      connectionRequests,
		MediationRequests:       mediationRequests,
		Drafts:                  drafts,
	}

	return mo.Ok(controlsPage)
}

type WritePage struct {
	*BasePage
	Prompt *postops.PostPrompt
}

func Write(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*WritePage] {
	dbUser := userData.DBUser
	var prompt *postops.PostPrompt
	var err error

	if promptID := c.Query("prompt"); promptID != "" {
		prompt, err = postops.GetPostPrompt(c, db,
			core.PostPromptWhere.RecipientID.EQ(dbUser.ID),
			core.PostPromptWhere.ID.EQ(promptID),
		)

		if err != nil {
			return mo.Err[*WritePage](err)
		}
	}

	writePage := &WritePage{
		BasePage: getBasePage(c, "New Post", userData),
		Prompt:   prompt,
	}

	return mo.Ok(writePage)
}

type SettingsPage struct {
	*BasePage
	AvailableInvites int64
	UsedInvites      core.UserInvitationSlice
	ActiveAPIKey     *core.UserAPIKey
	GeneralSettings  *forms.SettingsGeneralForm
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

	settingsPage := &SettingsPage{
		BasePage:         getBasePage(c, "Settings", userData),
		AvailableInvites: totalInvites - int64(len(usedInvites)),
		UsedInvites:      usedInvites,
		ActiveAPIKey:     apiKey,
		GeneralSettings:  forms.SettingsGeneralFormNew(userData.DBUser),
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

type SharedPostPage struct {
	*BasePage
	Author      *core.User
	Post        *core.Post
	PostSubject string
}

func SharedPost(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData, shareID string) mo.Result[*SharedPostPage] {
	share, err := core.PostShares(
		core.PostShareWhere.ID.EQ(shareID),
		qm.Load(qm.Rels(
			core.PostShareRels.Post,
			core.PostRels.User,
		)),
	).One(c, db)

	if err == sql.ErrNoRows {
		return mo.Err[*SharedPostPage](ginhelpers.ErrNotFound)
	} else if err != nil {
		return mo.Err[*SharedPostPage](err)
	}

	post := share.R.Post

	// drafts are not visible
	if post.PublishedAt.IsZero() {
		return mo.Err[*SharedPostPage](ginhelpers.ErrNotFound)
	}

	author := post.R.User

	sharedPost := &SharedPostPage{
		BasePage:    getBasePage(c, cmp.Or(post.Subject, "No Subject"), userData),
		Post:        post,
		PostSubject: cmp.Or(post.Subject, "No Subject"),
		Author:      author,
	}

	return mo.Ok(sharedPost)
}

type SinglePostPage struct {
	*BasePage
	Post      *postops.Post
	PostShare *core.PostShare
	Comments  []*postops.Comment
}

func SinglePost(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData, postID string, editPreview bool) mo.Result[*SinglePostPage] {
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

	var visitorID string

	if userData.DBUser != nil {
		visitorID = userData.DBUser.ID
	}

	connectionRadius, err := userops.GetConnectionRadius(c, db, visitorID, author.ID)

	if err != nil && err != userops.ErrUserNotSignedIn {
		return mo.Err[*SinglePostPage](err)
	}

	if !postops.CanSeePost(post, connectionRadius) {
		// no need to expose the fact that the post exists, hence 404
		return mo.Err[*SinglePostPage](ginhelpers.ErrNotFound)
	}

	constructed := postops.ConstructPost(userData.DBUser, post, connectionRadius, nil, editPreview)

	singlePostPage := &SinglePostPage{
		BasePage: getBasePage(c, constructed.PostSubject(), userData),
		Post:     constructed,
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

	if singlePostPage.Post.Capabilities.CanShare {
		postShare, err := core.PostShares(
			core.PostShareWhere.PostID.EQ(constructed.ID),
		).One(c, db)

		if err != nil && err != sql.ErrNoRows {
			return mo.Err[*SinglePostPage](err)
		}

		singlePostPage.PostShare = postShare
	}

	return mo.Ok(singlePostPage)
}

type EditPostPage struct {
	*BasePage
	PostID        string
	Input         forms.PostFormInput
	LastUpdatedAt time.Time
	IsPublished   bool
	Prompt        *postops.PostPrompt
}

func EditPost(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData, postID string) mo.Result[*EditPostPage] {
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
	title := "Edit Post"

	connectionRadius, err := userops.GetConnectionRadius(c, db, userData.DBUser.ID, author.ID)

	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	capabilities := postops.GetPostCapabilities(connectionRadius)

	if !capabilities.CanEdit {
		return mo.Err[*EditPostPage](ginhelpers.ErrForbidden)
	}

	prompt, err := postops.GetPostPrompt(c, db, core.PostPromptWhere.PostID.EQ(null.StringFrom(post.ID)))

	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	editPostPage := &EditPostPage{
		BasePage: getBasePage(c, title, userData),
		PostID:   post.ID,
		Input: forms.PostFormInput{
			Subject:    post.Subject,
			Body:       post.Body,
			Visibility: post.VisibilityRadius,
		},
		LastUpdatedAt: post.UpdatedAt.Time,
		IsPublished:   post.PublishedAt.Valid,
		Prompt:        prompt,
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

func UserHome(ctx *gin.Context, db boil.ContextExecutor, userData *auth.UserData, authorUsername string) mo.Result[*UserHomePage] {
	author, err := core.Users(
		core.UserWhere.Username.EQ(authorUsername),
	).One(ctx, db)

	if err == sql.ErrNoRows {
		return mo.Err[*UserHomePage](ginhelpers.ErrNotFound)
	} else if err != nil {
		return mo.Err[*UserHomePage](err)
	}

	if userops.CannotSeeProfileLite(author, userData.DBUser) {
		// don't wnat to tell whether a blog exists in the first place
		return mo.Err[*UserHomePage](ginhelpers.ErrNotFound)
	}

	var visitorID string
	if userData.DBUser != nil {
		visitorID = userData.DBUser.ID
	}

	connRadius, err := userops.GetConnectionRadius(ctx, db, visitorID, author.ID)

	if err != nil && err != userops.ErrUserNotSignedIn {
		return mo.Err[*UserHomePage](err)
	}

	if !userops.CanSeeProfile(author, userData.DBUser, connRadius) {
		// don't wnat to tell whether a blog exists in the first place
		return mo.Err[*UserHomePage](ginhelpers.ErrNotFound)
	}

	m := []qm.QueryMod{
		core.PostWhere.UserID.EQ(author.ID),
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Load(core.PostRels.User),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	}

	switch connRadius {
	case userops.ConnectionRadiusUnknown:
		// anon users get public posts only
		m = append(m, core.PostWhere.VisibilityRadius.IN([]core.PostVisibility{core.PostVisibilityPublic}))
	case userops.ConnectionRadiusSecondDegree:
		// second degree gets public and second degree posts
		m = append(m, core.PostWhere.VisibilityRadius.IN([]core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}))
	case userops.ConnectionRadiusDirect:
		// direct users inlcuding the author have no restrictions
		fallthrough
	case userops.ConnectionRadiusSameUser:
		m = append(m, qm.Load(core.PostRels.PostStat))
	}

	rawPosts, err := core.Posts(m...).All(ctx, db)

	if err != nil {
		return mo.Err[*UserHomePage](err)
	}

	posts := lo.Map(rawPosts, func(p *core.Post, idx int) *postops.Post {
		return postops.ConstructPost(userData.DBUser, p, connRadius, nil, false)
	})

	var isConnectionAllowed bool
	var mediationRequest *core.UserConnectionMediationRequest

	if userData.DBUser != nil {
		isConnectionAllowed, err = userops.IsConnectionAllowed(ctx, db, userData.DBUser.ID, author.ID)

		if err != nil {
			return mo.Err[*UserHomePage](err)
		}

		if connRadius == userops.ConnectionRadiusSecondDegree {
			mediationRequest, err = userops.GetMediationRequest(ctx, db, userData.DBUser.ID, author.ID)

			if err != nil {
				return mo.Err[*UserHomePage](err)
			}
		}
	}

	userHomePage := &UserHomePage{
		BasePage:          getBasePage(ctx, "Journal", userData),
		Author:            author,
		ConnectionRadius:  connRadius,
		ConnectionAllowed: isConnectionAllowed,
		MediationRequest:  mediationRequest,
		Posts:             posts,
	}

	return mo.Ok(userHomePage)
}

type FeedItem struct {
	Post    *postops.Post
	Comment *postops.Comment
}

func (fi *FeedItem) PublishedAt() time.Time {
	if fi.Post != nil {
		return fi.Post.PublishedAt.Time

	}

	return fi.Comment.CreatedAt
}

type FeedPage struct {
	*BasePage
	DirectConnections []*core.User
	OpenPrompts       []*postops.PostPrompt
	Items             []*FeedItem
}

func Feed(ctx *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*FeedPage] {
	user := userData.DBUser
	title := "Your Feed"

	directUserIDs, secondDegreeUserIDs, via, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	directMap := lo.KeyBy(directUserIDs, func(u string) string { return u })
	secondDegreeMap := lo.KeyBy(secondDegreeUserIDs, func(u string) string { return u })

	posts, err := core.Posts(
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Expr(
			core.PostWhere.UserID.IN(directUserIDs),
			qm.Or2(qm.Expr(
				core.PostWhere.UserID.IN(secondDegreeUserIDs),
				core.PostWhere.VisibilityRadius.EQ(core.PostVisibilitySecondDegree),
			))),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	seenUserIDs := lo.Filter(lo.Uniq(
		lo.Map(posts, func(p *core.Post, idx int) string { return p.UserID }),
	), func(id string, idx int) bool {
		if _, ok := secondDegreeMap[id]; ok {
			return true
		}

		return false
	})

	viaUserIDs := lo.Uniq(lo.FlatMap(seenUserIDs, func(id string, idx int) []string { return via[id] }))
	viaUsers, err := core.Users(
		core.UserWhere.ID.IN(viaUserIDs),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.UserColumns.CreatedAt)),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	viaUserMap := lo.KeyBy(viaUsers, func(u *core.User) string { return u.ID })

	items := lo.Map(posts, func(p *core.Post, idx int) *FeedItem {
		radius := userops.ConnectionRadiusSecondDegree
		var viaUsers []*core.User

		if _, ok := directMap[p.UserID]; ok {
			radius = userops.ConnectionRadiusDirect
		} else {
			viaUsers = lo.Map(via[p.UserID], func(id string, idx int) *core.User { return viaUserMap[id] })
		}

		return &FeedItem{
			Post: postops.ConstructPost(userData.DBUser, p, radius, viaUsers, false),
		}
	})

	comments, err := getComments(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	items = append(items, comments...)

	// newest items first
	slices.SortFunc(items, func(a, b *FeedItem) int {
		return b.PublishedAt().Compare(a.PublishedAt())
	})

	directConnections, err := core.Users(
		core.UserWhere.ID.IN(directUserIDs),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	dbPrompts, err := core.PostPrompts(
		core.PostPromptWhere.RecipientID.EQ(user.ID),
		core.PostPromptWhere.DismissedAt.IsNull(),
		qm.Load(core.PostPromptRels.Asker),
		qm.Load(core.PostPromptRels.Post),
		qm.OrderBy("? DESC", core.PostPromptColumns.ID),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	prompts := lo.Map(dbPrompts, func(p *core.PostPrompt, idx int) *postops.PostPrompt {
		return &postops.PostPrompt{
			Prompt: p,
			Author: p.R.Asker,
			Post:   p.R.Post,
		}
	})

	feedPage := &FeedPage{
		BasePage:          getBasePage(ctx, title, userData),
		DirectConnections: directConnections,
		OpenPrompts:       prompts,
		Items:             items,
	}

	return mo.Ok(feedPage)
}

func getComments(ctx context.Context, db boil.ContextExecutor, userID string) ([]*FeedItem, error) {
	// we want to add the comments from the posts
	// where the user has participated
	ownComments, err := core.PostComments(
		core.PostCommentWhere.UserID.EQ(userID),
		qm.Distinct(core.PostCommentColumns.PostID),
	).All(ctx, db)

	if err != nil {
		return nil, err
	}

	participatedPostIDs := lo.Map(ownComments, func(c *core.PostComment, idx int) string {
		return c.PostID
	})

	// we need to have this check there, since it could happen
	// that user has lost the connection to another user and
	// he has left a comment in one of their posts
	directUserIDs, err := userops.GetDirectUserIDs(ctx, db, userID)

	if err != nil {
		return nil, err
	}

	posts, err := core.Posts(
		qm.Expr(
			core.PostWhere.UserID.EQ(userID),
			qm.Or2(
				qm.Expr(
					core.PostWhere.UserID.IN(directUserIDs),
					core.PostWhere.ID.IN(participatedPostIDs),
				))),
		qm.Load(core.PostRels.User),
	).All(ctx, db)

	if err != nil {
		return nil, err
	}

	postMap := lo.KeyBy(posts, func(p *core.Post) string { return p.ID })

	comments, err := core.PostComments(
		core.PostCommentWhere.UserID.NEQ(userID),
		core.PostCommentWhere.PostID.IN(lo.Map(posts, func(p *core.Post, idx int) string { return p.ID })),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostCommentColumns.CreatedAt)),
		qm.Load(core.PostCommentRels.User),
	).All(ctx, db)

	if err != nil {
		return nil, err
	}

	return lo.Map(comments, func(c *core.PostComment, idx int) *FeedItem {
		post := postMap[c.PostID]

		return &FeedItem{
			Comment: &postops.Comment{
				PostComment: c,
				Author:      c.R.User,
				Post: &postops.Post{
					Author: post.R.User,
					Post:   post,
				},
			},
		}
	}), nil
}

type LoginPage struct {
	*BasePage
}

func Login(c *gin.Context, db boil.ContextExecutor, userData *auth.UserData) *LoginPage {
	invitePage := &LoginPage{
		BasePage: getBasePage(c, "Login", userData),
	}

	return invitePage
}
