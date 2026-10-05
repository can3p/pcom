package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type SharedPostPage struct {
	*BasePage
	Author      *model.User
	Post        *model.Post
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
	PostShare *model.PostShare
	Comments  []*postops.Comment
}

// PostPage is the page of a single post, /posts/:id.
func PostPage(c *gin.Context, userData *auth.UserData, post *reading.Post) *SinglePostPage {
	return &SinglePostPage{
		BasePage:  getBasePage(c, post.Post.PostSubject(), userData),
		Post:      post.Post,
		PostShare: post.PostShare,
		Comments:  post.Comments,
	}
}

type UserHomePage struct {
	*BasePage
	Author            *model.User
	ConnectionRadius  graph.Radius
	ConnectionAllowed bool
	MediationRequest  *model.UserConnectionMediationRequest
	Posts             []*postops.Post
	About             string
	Next              string // cursor of the next page, empty on the last
}

// UserHome is a user's journal, /users/:username. Only a public profile
// advertises its RSS feed.
func UserHome(c *gin.Context, userData *auth.UserData, journal *reading.Journal) *UserHomePage {
	basePage := getBasePage(c, "Journal", userData)

	if journal.Author.ProfileVisibility == model.ProfileVisibilityPublic {
		basePage.RSSFeed = links.Link("public_blog_feed", journal.Author.Username)
	}

	return &UserHomePage{
		BasePage:          basePage,
		Author:            journal.Author,
		ConnectionRadius:  journal.ConnectionRadius,
		ConnectionAllowed: journal.ConnectionAllowed,
		MediationRequest:  journal.MediationRequest,
		Posts:             journal.Posts,
		About:             journal.About,
		Next:              journal.Next,
	}
}

type FeedItem = reading.FeedItem

type FeedPageCapabilities struct {
	ShowPromptForm bool
}

type FeedPage struct {
	*BasePage
	DirectConnections []*model.User
	OpenPrompts       []*postops.PostPrompt
	Items             []*FeedItem
	Next              string // cursor of the next page, empty on the last
	LoadMoreURL       string
	Capabilities      FeedPageCapabilities
}

// Feed is the user's feed, /feed.
func Feed(c *gin.Context, userData *auth.UserData, feed *reading.Feed) *FeedPage {
	basePage := getBasePage(c, "Your Feed", userData)

	if feed.FeedToken != nil {
		basePage.RSSFeed = links.Link("private_user_feed", feed.FeedToken.Token)
	}

	return &FeedPage{
		BasePage:          basePage,
		DirectConnections: feed.DirectConnections,
		OpenPrompts:       feed.OpenPrompts,
		Items:             feed.Items,
		Next:              feed.Next,
		LoadMoreURL:       links.Link("feed"),
		Capabilities:      FeedPageCapabilities{ShowPromptForm: true},
	}
}

// Explore is /explore: posts anyone may read, without comments or actions.
func Explore(c *gin.Context, userData *auth.UserData, posts []*postops.Post) *FeedPage {
	return &FeedPage{
		BasePage: getBasePage(c, "Explore", userData),
		Items: lo.Map(posts, func(p *postops.Post, _ int) *FeedItem {
			return &FeedItem{Post: p}
		}),
		LoadMoreURL:  links.Link("explore"),
		Capabilities: FeedPageCapabilities{ShowPromptForm: false},
	}
}
