package web

import (
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

type WritePage struct {
	*BasePage
	Prompt *postops.PostPrompt
	// Proofread is set by the route when the grammar check and style review
	// are on: the editor then shows their buttons and panel.
	Proofread bool
	// Subscribers is what the form says about the author's email
	// subscribers.
	Subscribers *posts.SubscriberReach
	// Input is the author's scratch pad for this page's prompt, nil when
	// they never wrote one. A pad that was emptied still gives the revision
	// the form must name when it writes the pad again.
	Input *forms.PostFormInput
	// ScratchKeptAt is when the pad's text was kept, nil when the pad holds
	// none.
	ScratchKeptAt *time.Time
	// ScratchNotice is set while the author has not dismissed the notice
	// that a new post is kept as a scratch pad until it is saved as a draft.
	ScratchNotice bool
}

// scratchInput is the form input that shows the pad.
func scratchInput(pad *model.PostScratch) *forms.PostFormInput {
	in := &forms.PostFormInput{
		Subject:           lo.FromPtr(pad.Subject),
		URL:               lo.FromPtr(pad.URL),
		Body:              pad.Body,
		Visibility:        pad.Visibility,
		NotifyOffered:     pad.NotifySubscribers != nil,
		NotifySubscribers: lo.FromPtr(pad.NotifySubscribers),
		ScratchRev:        pad.Revision,
		NoteIDs:           strings.Join(pad.NoteIDs, ","),
	}

	if pad.IsEmpty() {
		// nothing was typed: the form's defaults, with the pad's revision
		in = &forms.PostFormInput{ScratchRev: pad.Revision, Visibility: model.PostVisibilitySecondDegree}
	}

	return in
}

// Write is the new post page. The prompt query parameter names the prompt the
// post answers; a prompt addressed to somebody else is ignored. The editor
// shows the author's scratch pad for that prompt.
func Write(c *gin.Context, svc *posts.Service, accountsSvc *accounts.Service, userData *auth.UserData) mo.Result[*WritePage] {
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

	pad, err := svc.Scratch(c, userData.DBUser, c.Query("prompt"))
	if err != nil {
		return mo.Err[*WritePage](err)
	}

	showNotice, err := accountsSvc.NoticeShown(c, userData.DBUser, accounts.NoticeScratchPad)
	if err != nil {
		return mo.Err[*WritePage](err)
	}

	page := &WritePage{
		BasePage:      getBasePage(c, "New Post", userData),
		Prompt:        prompt,
		Subscribers:   reach,
		ScratchNotice: showNotice,
	}

	if pad != nil {
		page.Input = scratchInput(pad)

		if !pad.IsEmpty() {
			page.ScratchKeptAt = &pad.UpdatedAt
		}
	}

	return mo.Ok(page)
}

// WritePreview is the preview of the author's scratch pad, the text of a new
// post that is not a draft yet, drawn as the post page draws a draft in
// edit-preview mode. An empty pad has nothing to preview.
func WritePreview(c *gin.Context, svc *posts.Service, userData *auth.UserData) mo.Result[*SinglePostPage] {
	user := userData.DBUser

	pad, err := svc.Scratch(c, user, c.Query("prompt"))
	if err != nil {
		return mo.Err[*SinglePostPage](err)
	}

	if pad == nil || pad.IsEmpty() {
		return mo.Err[*SinglePostPage](service.ErrNotFound)
	}

	post := &model.Post{
		Subject:          pad.Subject,
		Body:             pad.Body,
		UserID:           user.ID,
		VisibilityRadius: pad.Visibility,
		User:             user,
	}

	if url := lo.FromPtr(pad.URL); url != "" {
		post.URL = &model.NormalizedURL{URL: url}
	}

	view := postops.ConstructPost(user, post, graph.RadiusSameUser, nil, true)

	return mo.Ok(&SinglePostPage{
		BasePage: getBasePage(c, view.PostSubject(), userData),
		Post:     view,
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
		Subscribers:   reach,
	})
}

// NotePickerItem is a note of the panel; Text is what Insert and Append put
// into the post: the body, and under a quote the attribution line.
type NotePickerItem struct {
	Item *NoteItem
	Text string
	// Attached says the note belongs to the post the panel is for; Via says
	// a tag attached to the post brings it, so Detach hides it.
	Attached bool
	Via      bool
}

// NotePickerPage is the fragment of the post editor's Notes panel: a page of
// the notes the panel's search box (q, read like the notes list's) finds.
type NotePickerPage struct {
	Query NoteQuery
	// PostID is the post the panel attaches notes to. A new post that is not
	// a draft yet has none, and the panel says to save it first.
	PostID string
	// TopTags are the author's most used tags, which the panel offers under
	// its search box.
	TopTags []notes.TagCount
	// AttachTags are the tags of the search the panel can attach to the post.
	AttachTags []AttachTag
	Items      []*NotePickerItem
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

	line := capture.Attribution(n.Source.Title)
	if line == "" {
		return n.Body
	}

	return strings.TrimRight(n.Body, "\n") + "\n\n" + line
}

// NotePicker is the fragment of the Notes panel for the search of the request,
// and for the post of the request (post=), whose notes it shows first.
func NotePicker(c *gin.Context, svc *notes.Service, userData *auth.UserData) mo.Result[*NotePickerPage] {
	user := userData.DBUser
	q := ParseNoteQuery(c)
	q = NoteQuery{Q: q.Q, PostID: q.PostID}
	cursor := c.Query("cursor")

	var mine *notes.PostNotes

	if q.PostID != "" {
		if _, err := uuid.Parse(q.PostID); err != nil {
			return mo.Err[*NotePickerPage](service.ErrNotFound)
		}

		var err error

		if mine, err = svc.ForPost(c, user, q.PostID); err != nil {
			return mo.Err[*NotePickerPage](err)
		}
	}

	page, err := svc.List(c, user, q.Filter(cursor))
	if err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	top, err := svc.TopTags(c, user)
	if err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	out := &NotePickerPage{Query: q, TopTags: top, PostID: q.PostID}

	attached := map[string]notes.AttachedNote{}

	if mine != nil {
		for _, row := range mine.Notes {
			attached[row.Note.ID] = row
		}
	}

	// the tags of the search can be attached; without a post the controls
	// only say why they are off
	if len(q.Search().Tags) > 0 {
		all, err := svc.Tags(c, user)
		if err != nil {
			return mo.Err[*NotePickerPage](err)
		}

		out.AttachTags = attachTags(q.Search(), all)

		for i := range out.AttachTags {
			out.AttachTags[i].Done = mine != nil && slices.Contains(mine.Tags, out.AttachTags[i].Tag)
		}
	}

	// the post's own notes lead the first page of the whole list; a search
	// shows what it finds, the post's notes flagged
	lead := mine != nil && q.Q == ""

	var items []*NoteItem

	if lead && cursor == "" {
		for _, row := range mine.Notes {
			it, err := NoteItemOf(c, svc, user, row.Note)
			if err != nil {
				return mo.Err[*NotePickerPage](err)
			}

			items = append(items, it)
		}
	}

	for _, it := range noteItems(svc, page.Entries) {
		if _, ok := attached[it.Note.ID]; !ok || !lead {
			items = append(items, it)
		}
	}

	if err := itemsAttached(c, svc, user, items); err != nil {
		return mo.Err[*NotePickerPage](err)
	}

	for _, it := range items {
		row, ok := attached[it.Note.ID]
		out.Items = append(out.Items, &NotePickerItem{
			Item: it, Text: quoteText(it.Note), Attached: ok, Via: ok && len(row.ViaTags) > 0,
		})
	}

	if page.Next != "" {
		more := q.Values()
		more.Set("cursor", page.Next)
		out.More = links.Link("notes_picker") + "?" + more.Encode()
	}

	return mo.Ok(out)
}

// InsertNote puts the viewer's note id at the end of the text the edit page
// opens with, as Insert in the editor's Notes panel would, and tells the
// editor to save and mark the note used.
func InsertNote(c *gin.Context, svc *notes.Service, userData *auth.UserData, page *EditPostPage, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return service.ErrNotFound
	}

	note, err := svc.Get(c, userData.DBUser, id)
	if err != nil {
		return err
	}

	text := quoteText(note)

	if body := strings.TrimRight(page.Input.Body, "\n"); body != "" {
		text = body + "\n\n" + text
	}

	page.Input.Body = text
	page.Input.NoteIDs = note.ID

	return nil
}

// PostNoteItem is a note of a post's Notes section.
type PostNoteItem struct {
	Item *NoteItem
	// InsertHref opens the post's editor with the note inserted at the end.
	InsertHref string
	// Exclude says Detach hides the note, which a tag attached to the post
	// brings; otherwise it takes back the attaching.
	Exclude bool
}

// PostNoteGroup is the notes that came to a post the same way: attached one
// by one, or by one of its tags.
type PostNoteGroup struct {
	Title string
	Items []*PostNoteItem
}

// PostNotesTag is a tag of the post's notes in the tag bar; Href shows the
// notes with it, or all of them when it is the one on show.
type PostNotesTag struct {
	Tag    string
	Href   string
	Active bool
}

// PostNotesPage is the Notes section of a post page (GET /posts/:id/notes),
// which only the author's page loads.
type PostNotesPage struct {
	*BasePage
	PostID string
	// Attached are the tags attached to the post, each with Detach.
	Attached []string
	// Tags is the tag bar, over all the notes of the post.
	Tags   []PostNotesTag
	Groups []*PostNoteGroup
	// Count is how many notes belong to the post, whatever the tag bar shows.
	Count int
}

// PostNotes is the Notes section of the viewer's post, narrowed to the notes
// carrying tag when it is not empty.
func PostNotes(c *gin.Context, svc *notes.Service, userData *auth.UserData, postID, tag string) mo.Result[*PostNotesPage] {
	user := userData.DBUser

	pn, err := svc.ForPost(c, user, postID)
	if err != nil {
		return mo.Err[*PostNotesPage](err)
	}

	tag = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(tag), "#"))

	out := &PostNotesPage{
		BasePage: getBasePage(c, "Notes", userData), PostID: postID, Attached: pn.Tags, Count: len(pn.Notes),
	}

	byTag := map[string]*PostNoteGroup{}
	one := &PostNoteGroup{Title: "Attached one by one"}
	groups := []*PostNoteGroup{one}

	for _, t := range pn.Tags {
		g := &PostNoteGroup{Title: "Tagged #" + t}
		byTag[t] = g
		groups = append(groups, g)
	}

	seen := map[string]bool{}

	for _, row := range pn.Notes {
		for _, nt := range row.Note.AllTags() {
			seen[nt.Tag] = true
		}

		if tag != "" && !slices.ContainsFunc(row.Note.AllTags(), func(nt model.NoteTag) bool { return nt.Tag == tag }) {
			continue
		}

		item := &PostNoteItem{
			Item:       noteItem(svc, row.Note, false),
			InsertHref: links.Link("edit_post", postID) + "?insert=" + row.Note.ID,
			Exclude:    len(row.ViaTags) > 0,
		}

		switch {
		case row.Attached:
			one.Items = append(one.Items, item)
		case len(row.ViaTags) > 0:
			byTag[row.ViaTags[0]].Items = append(byTag[row.ViaTags[0]].Items, item)
		}
	}

	for _, g := range groups {
		if len(g.Items) > 0 {
			out.Groups = append(out.Groups, g)
		}
	}

	for _, t := range slices.Sorted(maps.Keys(seen)) {
		href := links.Link("post_notes", postID)
		if t != tag {
			href += "?tag=" + url.QueryEscape(t)
		}

		out.Tags = append(out.Tags, PostNotesTag{Tag: t, Href: href, Active: t == tag})
	}

	return mo.Ok(out)
}

// PostPageWithNotes is the page of a single post for its author, who gets
// the Comments | Notes toggle. Nobody else's page has a notes field set, and
// the toggle shows only for the author's.
type PostPageWithNotes struct {
	*SinglePostPage
	// NotesCount is how many notes belong to the post.
	NotesCount int
}

// WithPostNotes adds what the author's toggle needs to the post page: how many
// notes the post has. The notes themselves are loaded by the page on demand.
func WithPostNotes(c *gin.Context, svc *notes.Service, userData *auth.UserData, page *SinglePostPage) (*PostPageWithNotes, error) {
	out := &PostPageWithNotes{SinglePostPage: page}

	if !page.Post.Capabilities.CanEdit || page.Post.EditPreview {
		return out, nil
	}

	pn, err := svc.ForPost(c, userData.DBUser, page.Post.ID)
	if err != nil {
		return nil, err
	}

	out.NotesCount = len(pn.Notes)

	return out, nil
}
