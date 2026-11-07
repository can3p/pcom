package web

import (
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

type WritePage struct {
	*BasePage
	Prompt *postops.PostPrompt
	// Proofread is set by the route when the grammar check and style review
	// are on: the editor then shows their buttons and panel.
	Proofread bool
	// NotesEnabled shows the editor's Notes button and panel.
	NotesEnabled bool
	// Subscribers is what the form says about the author's email
	// subscribers.
	Subscribers *posts.SubscriberReach
}

// Write is the new post page. The prompt query parameter names the prompt the
// post answers; a prompt addressed to somebody else is ignored.
func Write(c *gin.Context, svc *posts.Service, userData *auth.UserData) mo.Result[*WritePage] {
	var prompt *postops.PostPrompt

	if promptID := c.Query("prompt"); promptID != "" {
		var err error

		prompt, err = svc.PromptFor(c, userData.DBUser, promptID)
		if err != nil {
			return mo.Err[*WritePage](err)
		}
	}

	reach, err := svc.SubscriberReach(c, userData.DBUser, nil)
	if err != nil {
		return mo.Err[*WritePage](err)
	}

	return mo.Ok(&WritePage{
		BasePage:     getBasePage(c, "New Post", userData),
		Prompt:       prompt,
		NotesEnabled: true,
		Subscribers:  reach,
	})
}

type EditPostPage struct {
	*BasePage
	PostID        string
	Input         forms.PostFormInput
	LastUpdatedAt time.Time
	IsPublished   bool
	Prompt        *postops.PostPrompt
	// Proofread is set by the route, as on WritePage.
	Proofread bool
	// NotesEnabled is as on WritePage.
	NotesEnabled bool
	// Subscribers is as on WritePage, for this post.
	Subscribers *posts.SubscriberReach
}

// EditPost is the edit page of a post the viewer may edit.
func EditPost(c *gin.Context, svc *posts.Service, userData *auth.UserData, postID string) mo.Result[*EditPostPage] {
	view, err := svc.ForEdit(c, userData.DBUser, postID)
	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	post := view.Post

	reach, err := svc.SubscriberReach(c, userData.DBUser, post)
	if err != nil {
		return mo.Err[*EditPostPage](err)
	}

	var url string

	if post.URL != nil {
		url = post.URL.URL
	}

	return mo.Ok(&EditPostPage{
		BasePage: getBasePage(c, "Edit Post", userData),
		PostID:   post.ID,
		Input: forms.PostFormInput{
			Subject:    lo.FromPtr(post.Subject),
			Body:       post.Body,
			Visibility: post.VisibilityRadius,
			URL:        url,
		},
		LastUpdatedAt: lo.FromPtr(post.UpdatedAt),
		IsPublished:   post.PublishedAt != nil,
		Prompt:        view.Prompt,
		NotesEnabled:  true,
		Subscribers:   reach,
	})
}

// NotePickerPath is the fragment the post editor's Notes panel loads.
const NotePickerPath = "/controls/notes/picker"

// NotePickerTarget is the element of the panel the fragment goes into.
const NotePickerTarget = "#np-body"

// NotePickerItem is a note of the panel; Text is what Insert and Append put
// into the post: the body, and under a quote the attribution line.
type NotePickerItem struct {
	Item *NoteItem
	Text string
}

// NotePickerPage is the fragment of the post editor's Notes panel: the tag
// bar, the source filter and a page of notes.
type NotePickerPage struct {
	Query   NoteQuery
	Bar     *NoteTagBar
	Sources []NoteLink
	Items   []*NotePickerItem
	// More loads the next page, empty on the last.
	More string
}

// quoteText is the text a note puts into a post: the body, and for a note
// with a source (a quote, whose body is already a blockquote) the attribution
// line after it.
func quoteText(n *model.Note) string {
	if n.Source == nil {
		return n.Body
	}

	line := capture.Attribution(capture.Source{Author: derefString(n.Source.Author), Title: n.Source.Title, Page: derefString(n.Locator)})
	if line == "" {
		return n.Body
	}

	return strings.TrimRight(n.Body, "\n") + "\n\n" + line
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// NotePicker is the fragment of the Notes panel for the view of the request.
func NotePicker(c *gin.Context, svc *notes.Service, userData *auth.UserData) mo.Result[*NotePickerPage] {
	user := userData.DBUser
	q := ParseNoteQuery(c)

	tags, err := svc.Tags(c, user)
	if err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	page, err := svc.List(c, user, q.Filter(c.Query("cursor")))
	if err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	items, err := noteItems(svc, c, user, page.Entries, false)
	if err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	sources, err := svc.Sources(c, user)
	if err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	out := &NotePickerPage{Query: q, Bar: NewNoteTagBar(NotePickerPath, NotePickerTarget, q, tags)}

	for _, it := range items {
		out.Items = append(out.Items, &NotePickerItem{Item: it, Text: quoteText(it.Note)})
	}

	if page.Next != "" {
		more := q.Values()
		more.Set("cursor", page.Next)
		out.More = NotePickerPath + "?" + more.Encode()
	}

	for _, s := range sources {
		on := s.Source.ID == q.SourceID

		next := s.Source.ID
		if on {
			next = ""
		}

		out.Sources = append(out.Sources, NoteLink{
			Label: s.Source.Title, Count: s.Notes, Active: on, Href: q.withSource(next).URL(NotePickerPath),
		})
	}

	return mo.Ok(out)
}
