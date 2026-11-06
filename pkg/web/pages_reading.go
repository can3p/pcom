package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/can3p/pcom/pkg/service/subscriptions"
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

// SubscribeOffer is the email subscription form a journal or a public post
// shows to a visitor without an account: whose posts it subscribes to, and the
// challenge the form carries (abuse.Challenge.Token). A page the visitor may
// not subscribe from has none.
type SubscribeOffer struct {
	Username  string
	Challenge string
}

type SinglePostPage struct {
	*BasePage
	Post      *postops.Post
	PostShare *model.PostShare
	Comments  []*postops.Comment
	// Subscribe is set when the subscribe form should be shown.
	Subscribe *SubscribeOffer
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
	// Subscribe is set when the subscribe form should be shown.
	Subscribe *SubscribeOffer
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

// SubscriptionConfirmPage is the page a confirmation link opens.
type SubscriptionConfirmPage struct {
	*BasePage
	Token string
	// Author is the journal the link subscribes to.
	Author string
	// Email is the address the link subscribes.
	Email string
	// Paused is set when the author takes no subscriptions right now.
	Paused bool
	// Done is set once the subscription is confirmed.
	Done bool
	// Expired is set for an unknown or expired link.
	Expired bool
}

// SubscriptionConfirm is the page of a confirmation link: asking to confirm,
// or (done) confirmed; a nil view is a link that has expired.
func SubscriptionConfirm(c *gin.Context, userData *auth.UserData, token string, view *subscriptions.ConfirmView, done bool) *SubscriptionConfirmPage {
	page := &SubscriptionConfirmPage{
		BasePage: getBasePage(c, "Confirm your subscription", userData),
		Token:    token,
		Done:     done,
		Expired:  view == nil,
	}

	if view != nil {
		page.Author, page.Email, page.Paused = view.AuthorUsername, view.Email, view.Paused
	}

	return page
}

// SubscriptionsManagePage is the page of a manage link.
type SubscriptionsManagePage struct {
	*BasePage
	Token string
	// View is nil for an unknown link, or when nothing is left to manage.
	View *subscriptions.ManageView
	// Notice tells what the request just made did.
	Notice string
	// Expired is set for an unknown link.
	Expired bool
}

// SubscriptionsManage is the manage page; a nil view without a notice is an
// unknown link.
func SubscriptionsManage(c *gin.Context, userData *auth.UserData, token string, view *subscriptions.ManageView, notice string) *SubscriptionsManagePage {
	return &SubscriptionsManagePage{
		BasePage: getBasePage(c, "Your email subscriptions", userData),
		Token:    token,
		View:     view,
		Notice:   notice,
		Expired:  view == nil && notice == "",
	}
}

// SubscriptionsLostPage is the form of a subscriber who lost the link to
// their subscriptions.
type SubscriptionsLostPage struct {
	*BasePage
	Challenge string
}

// SubscriptionsLost is the lost-link page, carrying the form's challenge.
func SubscriptionsLost(c *gin.Context, userData *auth.UserData, challenge string) *SubscriptionsLostPage {
	return &SubscriptionsLostPage{
		BasePage:  getBasePage(c, "Your email subscriptions", userData),
		Challenge: challenge,
	}
}
