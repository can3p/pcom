package web

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

type SharedPostPage struct {
	*BasePage
	Author      *core.User
	Post        *core.Post
	PostSubject string
}

// SharedPost is the page a share link shows.
func SharedPost(c *gin.Context, userData *auth.UserData, shared *shares.Shared) *SharedPostPage {
	subject := postops.PostSubject(shared.Post.Subject)

	return &SharedPostPage{
		BasePage:    getBasePage(c, subject, userData),
		Post:        shared.Post,
		PostSubject: subject,
		Author:      shared.Author,
	}
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
		qm.Load(core.PostRels.URL),
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
		if !userData.IsLoggedIn {
			return mo.Err[*SinglePostPage](ginhelpers.ErrNeedsLogin)
		}

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
			qm.OrderBy(fmt.Sprintf("%s ASC", core.PostCommentColumns.CreatedAt))).All(c, db)

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
		// don't want to tell whether a blog exists in the first place
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
		// don't want to tell whether a blog exists in the first place
		return mo.Err[*UserHomePage](ginhelpers.ErrNotFound)
	}

	m := []qm.QueryMod{
		core.PostWhere.UserID.EQ(author.ID),
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.URL),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	}

	switch connRadius {
	case userops.ConnectionRadiusDirect:
		// direct users including the author have no restrictions
		fallthrough
	case userops.ConnectionRadiusSameUser:
		m = append(m, qm.Load(core.PostRels.PostStat))
	case userops.ConnectionRadiusSecondDegree:
		// second degree gets public and second degree posts
		m = append(m, core.PostWhere.VisibilityRadius.IN([]core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}))
	default:
		// anonymous and unrelated visitors, and any radius added later, get
		// public posts only
		m = append(m, core.PostWhere.VisibilityRadius.IN([]core.PostVisibility{core.PostVisibilityPublic}))
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

	basePage := getBasePage(ctx, "Journal", userData)

	if author.ProfileVisibility == core.ProfileVisibilityPublic {
		basePage.RSSFeed = links.Link("public_blog_feed", author.Username)
	}

	userHomePage := &UserHomePage{
		BasePage:          basePage,
		Author:            author,
		ConnectionRadius:  connRadius,
		ConnectionAllowed: isConnectionAllowed,
		MediationRequest:  mediationRequest,
		Posts:             posts,
	}

	return mo.Ok(userHomePage)
}

type FeedItem struct {
	Post     *postops.Post
	FeedItem *feedops.RssFeedItem
	Comment  *postops.Comment
}

type FeedPageCapabilities struct {
	ShowPromptForm bool
}

type FeedPage struct {
	*BasePage
	DirectConnections []*core.User
	OpenPrompts       []*postops.PostPrompt
	Items             []*FeedItem
	Capabilities      FeedPageCapabilities
}

func (i *FeedItem) AddedToFeedAt() time.Time {
	if i.Post != nil {
		return i.Post.PublishedAt.Time
	}

	if i.FeedItem != nil {
		return i.FeedItem.AddedAt
	}

	return i.Comment.CreatedAt
}

func Feed(ctx *gin.Context, db boil.ContextExecutor, userData *auth.UserData, onlyPosts bool) mo.Result[*FeedPage] {
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
				core.PostWhere.VisibilityRadius.IN([]core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}),
			))),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
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

	if onlyPosts {
		feedPage := &FeedPage{
			Items: items,
		}

		return mo.Ok(feedPage)
	}

	rssFeedItems, err := feedops.GetRssFeedItems(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	rssFeedItemsMapped := lo.Map(rssFeedItems, func(p *feedops.RssFeedItem, idx int) *FeedItem {
		return &FeedItem{
			FeedItem: p,
		}
	})

	items = append(items, rssFeedItemsMapped...)

	comments, err := getComments(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	items = append(items, comments...)

	// newest items first
	slices.SortFunc(items, func(a, b *FeedItem) int {
		return b.AddedToFeedAt().Compare(a.AddedToFeedAt())
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
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostPromptColumns.CreatedAt)),
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

	basePage := getBasePage(ctx, title, userData)

	feedToken, err := repo.FeedTokenForUser(ctx, db, user.ID)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	if feedToken != nil {
		basePage.RSSFeed = links.Link("private_user_feed", feedToken.Token)
	}

	feedPage := &FeedPage{
		BasePage:          basePage,
		DirectConnections: directConnections,
		OpenPrompts:       prompts,
		Items:             items,
		Capabilities:      FeedPageCapabilities{ShowPromptForm: true},
	}

	return mo.Ok(feedPage)
}

func Explore(ctx *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*FeedPage] {
	title := "Explore"

	visibleProfiles := []core.ProfileVisibility{core.ProfileVisibilityPublic}

	if userData.IsLoggedIn {
		// we're not handling connections visibility there, since
		// these posts will be visible in the usual feed anyway
		visibleProfiles = append(visibleProfiles, core.ProfileVisibilityRegisteredUsers)
	}

	posts, err := core.Posts(
		core.PostWhere.PublishedAt.IsNotNull(),
		core.PostWhere.VisibilityRadius.EQ(core.PostVisibilityPublic),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
		qm.LeftOuterJoin("users on users.ID = posts.user_id"),
		core.UserWhere.ProfileVisibility.IN(visibleProfiles),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*FeedPage](err)
	}

	items := lo.Map(posts, func(p *core.Post, idx int) *FeedItem {
		return &FeedItem{
			Post: postops.ConstructPost(userData.DBUser, p, userops.ConnectionRadiusUnknown, nil, false),
		}
	})

	basePage := getBasePage(ctx, title, userData)

	feedPage := &FeedPage{
		BasePage:     basePage,
		Items:        items,
		Capabilities: FeedPageCapabilities{ShowPromptForm: false},
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
