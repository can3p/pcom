package web

import (
	"context"
	"slices"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
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
	// Translation is nil until Translate is called, and for anonymous readers.
	Translation *TranslationView
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
	Author            *core.User
	ConnectionRadius  graph.Radius
	ConnectionAllowed bool
	MediationRequest  *core.UserConnectionMediationRequest
	Posts             []*postops.Post
	About             string
	Next              string // cursor of the next page, empty on the last
	Translation       *TranslationView
}

// UserHome is a user's journal, /users/:username. Only a public profile
// advertises its RSS feed.
func UserHome(c *gin.Context, userData *auth.UserData, journal *reading.Journal) *UserHomePage {
	basePage := getBasePage(c, "Journal", userData)

	if journal.Author.ProfileVisibility == core.ProfileVisibilityPublic {
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
	DirectConnections []*core.User
	OpenPrompts       []*postops.PostPrompt
	Items             []*FeedItem
	Next              string // cursor of the next page, empty on the last
	LoadMoreURL       string
	Capabilities      FeedPageCapabilities
	Translation       *TranslationView
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

// TranslationView is what a page shows of translation: for each post and RSS
// item on it, whether the reader gets a Translate button, and the cached
// translation of those in a language they always translate. It is built once
// per page; a nil view, for anonymous readers or with translation off, shows
// no control.
type TranslationView struct {
	items map[string]*translationItem
}

type translationItem struct {
	translatable bool
	auto         bool // the reader always translates its language
	result       *translations.Result
}

func translationKey(kind, id string) string { return kind + ":" + id }

// translationSource is a post or an RSS item on a page.
type translationSource struct {
	id           string
	translatable bool
	lang         string
}

// newTranslationView asks the service about the posts and RSS items of a
// page: one read of the reader's languages and one of the cache per kind.
func newTranslationView(ctx context.Context, svc *translations.Service, actor *core.User, posts, rssItems []translationSource) (*TranslationView, error) {
	if svc == nil || !svc.Enabled() || actor == nil {
		return nil, nil //nolint:nilnil // no view means no control
	}

	langs, err := svc.Languages(ctx, actor)
	if err != nil {
		return nil, err
	}

	view := &TranslationView{items: map[string]*translationItem{}}

	for _, k := range []struct {
		kind    string
		source  core.TranslationSourceKind
		sources []translationSource
	}{{"post", core.TranslationSourceKindPost, posts}, {"rss_item", core.TranslationSourceKindRSSItem, rssItems}} {
		var autoIDs []string

		for _, src := range k.sources {
			if !src.translatable {
				continue
			}

			item := &translationItem{translatable: true, auto: slices.Contains(langs, src.lang)}
			view.items[translationKey(k.kind, src.id)] = item

			if item.auto {
				autoIDs = append(autoIDs, src.id)
			}
		}

		if len(autoIDs) == 0 {
			continue
		}

		cached, err := svc.Cached(ctx, actor, k.source, autoIDs)
		if err != nil {
			return nil, err
		}

		for id, res := range cached {
			view.items[translationKey(k.kind, id)].result = res
		}
	}

	return view, nil
}

func postSources(svc *translations.Service, actor *core.User, posts []*postops.Post) []translationSource {
	var out []translationSource

	for _, p := range posts {
		if p != nil && p.Post != nil {
			out = append(out, translationSource{id: p.ID, translatable: svc.TranslatablePost(actor, p.Post), lang: p.Language.String})
		}
	}

	return out
}

// Translate builds the page's translation view.
func (p *FeedPage) Translate(ctx context.Context, svc *translations.Service) (err error) {
	var posts []*postops.Post

	var rssItems []translationSource

	for _, it := range p.Items {
		if it.Post != nil {
			posts = append(posts, it.Post)
		}

		if it.FeedItem != nil && svc != nil {
			rssItems = append(rssItems, translationSource{
				id: it.FeedItem.RSSItemID, lang: it.FeedItem.Language.String,
				translatable: svc.TranslatableRSSItem(p.User.DBUser, it.FeedItem.Language),
			})
		}
	}

	if svc == nil {
		return nil
	}

	p.Translation, err = newTranslationView(ctx, svc, p.User.DBUser, postSources(svc, p.User.DBUser, posts), rssItems)

	return err
}

// Translate builds the journal's translation view.
func (p *UserHomePage) Translate(ctx context.Context, svc *translations.Service) (err error) {
	if svc == nil {
		return nil
	}

	p.Translation, err = newTranslationView(ctx, svc, p.User.DBUser, postSources(svc, p.User.DBUser, p.Posts), nil)

	return err
}

// Translate builds the post page's translation view.
func (p *SinglePostPage) Translate(ctx context.Context, svc *translations.Service) (err error) {
	if svc == nil {
		return nil
	}

	p.Translation, err = newTranslationView(ctx, svc, p.User.DBUser, postSources(svc, p.User.DBUser, []*postops.Post{p.Post}), nil)

	return err
}

// Slot is the data of the "translation.slot" template for a post or an RSS
// item (kind "post" or "rss_item") shown in view "feed", "home" or "single":
// the original, with a Translate button when the reader may translate it,
// and in place of both the translation when it is cached and the reader
// always translates its language. An uncached one asks the server to
// translate on load. Nil-safe.
func (v *TranslationView) Slot(kind, id, view, body, lang string) map[string]any {
	slot := map[string]any{"Kind": kind, "ID": id, "View": view, "Body": body, "Lang": lang}

	var item *translationItem
	if v != nil {
		item = v.items[translationKey(kind, id)]
	}

	switch {
	case item == nil:
	case item.result != nil:
		slot["Translated"] = true
		slot["Subject"] = item.result.Subject
		slot["Body"] = item.result.Body
		slot["Language"] = forms.LanguageName(item.result.SourceLang)
		slot["Provider"] = item.result.Provider
	case item.auto:
		slot["Load"] = true
	default:
		slot["CanTranslate"] = true
	}

	return slot
}
