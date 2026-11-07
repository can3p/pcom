package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/util/date"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/mo"
)

// NotesPath is the notes list; the pages of the notes live under it.
const NotesPath = "/controls/notes"

// NoteQuery is the view of the notes list a URL describes: the tags a note
// must (Tags) and must not (Not) carry, whether hidden tags are shown, the
// source, the unused and Inbox filters and the text search. The tag bar and the
// post editor's panel build their links from it.
type NoteQuery struct {
	Tags, Not  []string
	ShowHidden bool
	SourceID   string
	// StackID is the stack the view shows alone (notes.Entry.StackID).
	StackID       string
	Unused, Inbox bool
	Q             string
}

// ParseNoteQuery reads a NoteQuery from the request's query string.
func ParseNoteQuery(c *gin.Context) NoteQuery {
	return NoteQuery{
		Tags:       c.QueryArray("tag"),
		Not:        c.QueryArray("not"),
		ShowHidden: c.Query("hidden") == "1",
		SourceID:   c.Query("source"),
		StackID:    c.Query("stack"),
		Unused:     c.Query("unused") == "1",
		Inbox:      c.Query("inbox") == "1",
		Q:          strings.TrimSpace(c.Query("q")),
	}
}

// Filter is the notes service's filter for q, from the page after cursor.
func (q NoteQuery) Filter(cursor string) notes.Filter {
	return notes.Filter{
		StackID: q.StackID,
		Tags:    q.Tags, NotTags: q.Not, ShowHidden: q.ShowHidden, SourceID: q.SourceID,
		Unused: q.Unused, Inbox: q.Inbox, Query: q.Q, Cursor: cursor,
	}
}

// Values is the query string of q.
func (q NoteQuery) Values() url.Values {
	v := url.Values{}

	for _, t := range q.Tags {
		v.Add("tag", t)
	}

	for _, t := range q.Not {
		v.Add("not", t)
	}

	if q.ShowHidden {
		v.Set("hidden", "1")
	}

	if q.SourceID != "" {
		v.Set("source", q.SourceID)
	}

	if q.StackID != "" {
		v.Set("stack", q.StackID)
	}

	if q.Unused {
		v.Set("unused", "1")
	}

	if q.Inbox {
		v.Set("inbox", "1")
	}

	if q.Q != "" {
		v.Set("q", q.Q)
	}

	return v
}

// URL is path with the query of q.
func (q NoteQuery) URL(path string) string {
	if enc := q.Values().Encode(); enc != "" {
		return path + "?" + enc
	}

	return path
}

// Cycle moves tag one step round include, exclude and off.
func (q NoteQuery) Cycle(tag string) NoteQuery {
	out := q
	out.Tags = slices.DeleteFunc(slices.Clone(q.Tags), func(t string) bool { return t == tag })
	out.Not = slices.DeleteFunc(slices.Clone(q.Not), func(t string) bool { return t == tag })

	switch {
	case slices.Contains(q.Tags, tag):
		out.Not = append(out.Not, tag)
	case slices.Contains(q.Not, tag):
	default:
		out.Tags = append(out.Tags, tag)
	}

	return out
}

// withInbox, withUnused and withSource are q with the one filter changed and
// the others kept.
func (q NoteQuery) withInbox(on bool) NoteQuery {
	q.Inbox = on

	return q
}

func (q NoteQuery) withUnused(on bool) NoteQuery {
	q.Unused = on

	return q
}

func (q NoteQuery) withSource(id string) NoteQuery {
	q.SourceID = id

	return q
}

// WithHidden is q with the hidden tags' notes shown or not.
func (q NoteQuery) WithHidden(show bool) NoteQuery {
	q.ShowHidden = show

	return q
}

// Filtered reports whether q narrows the list down at all.
func (q NoteQuery) Filtered() bool {
	return len(q.Tags)+len(q.Not) > 0 || q.SourceID != "" || q.StackID != "" || q.Unused || q.Inbox || q.Q != ""
}

// NoteChip is a tag of the tag bar: State is "include", "exclude" or "off",
// and Href the view a tap on it leads to.
type NoteChip struct {
	Tag    string
	Count  int
	State  string
	Hidden bool
	Href   string
}

// NoteTagBar is the tag bar of the notes list and the post editor's panel.
// A chip is a link to Path with the next state of the tag; with a Target
// the link is fetched in place into that element instead. Manage adds Hide
// and Unhide to the chips.
type NoteTagBar struct {
	Path   string
	Target string
	Manage bool
	Query  NoteQuery
	Chips  []NoteChip
	// HiddenTags is how many tags the author hides; ShowHiddenHref flips
	// whether their notes are in the view.
	HiddenTags     int
	ShowHiddenHref string
}

// NewNoteTagBar builds the tag bar of the tags for the view q at path.
func NewNoteTagBar(path, target string, manage bool, q NoteQuery, tags []notes.TagCount) *NoteTagBar {
	bar := &NoteTagBar{Path: path, Target: target, Manage: manage, Query: q}

	for _, t := range tags {
		if t.Hidden {
			bar.HiddenTags++

			if !q.ShowHidden && !slices.Contains(q.Tags, t.Tag) {
				continue
			}
		}

		state := "off"

		switch {
		case slices.Contains(q.Tags, t.Tag):
			state = "include"
		case slices.Contains(q.Not, t.Tag):
			state = "exclude"
		}

		bar.Chips = append(bar.Chips, NoteChip{
			Tag: t.Tag, Count: t.Count, State: state, Hidden: t.Hidden,
			Href: q.Cycle(t.Tag).URL(path),
		})
	}

	bar.ShowHiddenHref = q.WithHidden(!q.ShowHidden).URL(path)

	return bar
}

// NoteTag is a tag of a note; FromText says it comes from a #word of the
// body, so it goes with the text.
type NoteTag struct {
	Tag      string
	FromText bool
}

// NoteItem is a note of the list.
type NoteItem struct {
	Note *model.Note
	Used bool
	Tags []NoteTag
	// Suggestions are the tags the Inbox offers for the note.
	Suggestions []string
	// DeletePrompt is what the confirmation of its Delete says.
	DeletePrompt string
	// Status is the transcription status the item shows for a recording.
	Status *NoteStatus
	// Again is the "Transcribe again as…" control, nil when the note has none.
	Again *NoteAgain
}

// TranscribeLanguage is a language the "Transcribe again as…" picker offers:
// a Whisper language code and its name.
type TranscribeLanguage struct {
	Code string
	Name string
}

// TranscribeLanguages is the picker's fixed list, in the order it shows.
var TranscribeLanguages = []TranscribeLanguage{
	{"en", "English"}, {"ru", "Russian"}, {"nl", "Dutch"}, {"de", "German"}, {"fr", "French"},
	{"es", "Spanish"}, {"it", "Italian"}, {"pt", "Portuguese"}, {"uk", "Ukrainian"}, {"pl", "Polish"},
}

// confirmRetranscribe is what the control asks before it replaces text.
const confirmRetranscribe = "Replace the edited text with a new transcript?"

// NoteAgain is the "Transcribe again as…" control of a note with a recording
// (partial--note-again.html). Confirm is the question asked before sending,
// empty when there is no text to lose. The note doesn't keep its last
// transcript, so the rule is: a note with text confirms, and the edit page
// always does, since the text box may hold what was typed since it opened.
type NoteAgain struct {
	NoteID    string
	Languages []TranscribeLanguage
	Confirm   string
}

// NoteAgainOf is the control of n, nil when the note has no recording,
// transcription is off or the recording is being transcribed already.
func NoteAgainOf(svc *notes.Service, n *model.Note, edit bool) *NoteAgain {
	if n.AudioUploadID == nil || !svc.TranscriptionEnabled() || n.TranscriptionStatus == model.NoteTranscriptionPending {
		return nil
	}

	a := &NoteAgain{NoteID: n.ID, Languages: TranscribeLanguages}
	if edit || strings.TrimSpace(n.Body) != "" {
		a.Confirm = confirmRetranscribe
	}

	return a
}

// Where a note's transcription status shows; its poll answers for that place.
const (
	NoteStatusList = "list"
	NoteStatusEdit = "edit"
)

// NoteStatus is the transcription status of a note's recording, as
// partial--note-status.html shows it. View is NoteStatusList, NoteStatusEdit
// or "" for the status alone. On the list, Stack is the stack the note shows
// in and Suggest whether its tag suggestions show, so the item that replaces
// it once the text is there looks the same. On the edit page, Base is the
// body the page was opened with, which every poll sends back with the text
// being edited; Typed says the two differ, so the author changed the text
// while the recording was transcribed: the form keeps it and Transcript shows
// next to it instead.
type NoteStatus struct {
	Note       *model.Note
	View       string
	Stack      string
	Suggest    bool
	Base       string
	Typed      bool
	Transcript string
}

// Query is the query string of the status poll, which tells the handler
// where the status shows.
func (s *NoteStatus) Query() string {
	v := url.Values{}

	if s.View != "" {
		v.Set("view", s.View)
	}

	if s.Stack != "" {
		v.Set("stack", s.Stack)
	}

	if s.Suggest {
		v.Set("suggest", "1")
	}

	return v.Encode()
}

// NoteItemOf is the list item of the viewer's note n, as the list the status
// at shows in has it.
func NoteItemOf(c *gin.Context, svc *notes.Service, user *model.User, n *model.Note, at *NoteStatus) (*NoteItem, error) {
	links, err := svc.PostLinks(c, user, n.ID)
	if err != nil {
		return nil, err
	}

	item := &NoteItem{
		Note: n, Used: links > 0, Tags: NoteTagsOf(n),
		DeletePrompt: NoteDeletePrompt(n, links > 0),
		Status:       &NoteStatus{Note: n, View: NoteStatusList, Stack: at.Stack, Suggest: at.Suggest},
		Again:        NoteAgainOf(svc, n, false),
	}

	if at.Suggest {
		if item.Suggestions, err = svc.SuggestTags(c, user, n.ID); err != nil {
			return nil, err
		}
	}

	return item, nil
}

// TranscriptOf is what the transcription added to a note's body, given what
// the body may have been before it: the notes service appends the text after
// a blank line, or makes it the body of a note without text. A body that
// starts with none of them is all transcript, as far as can be told.
func TranscriptOf(body string, before ...string) string {
	for _, b := range before {
		b = strings.TrimRight(b, "\n")
		if strings.TrimSpace(b) != "" && strings.HasPrefix(body, b+"\n\n") {
			return strings.TrimPrefix(body, b+"\n\n")
		}
	}

	return body
}

// NoteGroup is the notes of a list under one heading: the notes of a stack
// of several, or a single note.
type NoteGroup struct {
	StackID string
	Stack   bool
	Label   string
	// Href is the view of the stack alone.
	Href  string
	Items []*NoteItem
}

// NoteLink is a view of the list the page links to; Active says it is the
// one on show.
type NoteLink struct {
	Label  string
	Count  int
	Href   string
	Active bool
}

// NotesPage is /controls/notes and its management screen.
type NotesPage struct {
	*BasePage
	Query  NoteQuery
	Bar    *NoteTagBar
	Groups []*NoteGroup
	// More is the link that loads the next page, empty on the last.
	More string
	// Focus is the author's current focus tag.
	Focus string
	// Views are the Inbox and Unused filters, Sources the views by source.
	Views   []NoteLink
	Sources []NoteLink
	// Manage is set on the screen that renames and merges tags and sources.
	Manage *NoteManage
}

// NoteManage is the tag and source management screen.
type NoteManage struct {
	Tags    []notes.TagCount
	Sources []notes.SourceCount
	Focus   string
}

// NoteDeletePrompt is the confirmation of deleting a note: what goes with it,
// and that the text already in posts stays.
func NoteDeletePrompt(n *model.Note, used bool) string {
	var parts []string

	if n.AudioUploadID != nil {
		parts = append(parts, "its recording")
	}

	switch len(n.Images) {
	case 0:
	case 1:
		parts = append(parts, "1 photo")
	default:
		parts = append(parts, fmt.Sprintf("%d photos", len(n.Images)))
	}

	s := "Delete this note"

	switch len(parts) {
	case 1:
		s += " and " + parts[0]
	case 2:
		s += ", " + parts[0] + " and " + parts[1]
	}

	s += "? This can't be undone."

	if used {
		s += " The text already inserted into posts stays there."
	}

	return s
}

// stackLabel is "Tuesday evening, 7 notes" for the stack captured at t.
func stackLabel(t time.Time, user *model.User, size int) string {
	at := date.LocalizeTime(user, t)

	part := "night"

	switch h := at.Hour(); {
	case h >= 5 && h < 12:
		part = "morning"
	case h >= 12 && h < 17:
		part = "afternoon"
	case h >= 17 && h < 22:
		part = "evening"
	}

	return fmt.Sprintf("%s %s, %d notes", at.Weekday(), part, size)
}

// NoteTagsOf lists the tags of a note, the ones set by hand first.
func NoteTagsOf(n *model.Note) []NoteTag {
	var out []NoteTag

	for _, t := range n.Tags {
		out = append(out, NoteTag{Tag: t})
	}

	for _, t := range n.TextTags {
		if !slices.Contains(n.Tags, t) {
			out = append(out, NoteTag{Tag: t, FromText: true})
		}
	}

	return out
}

// noteGroups puts the entries of a list, newest first, into groups: the
// neighbours of one stack form a group.
func noteGroups(svc *notes.Service, c *gin.Context, user *model.User, entries []notes.Entry, suggest bool) ([]*NoteGroup, error) {
	var groups []*NoteGroup

	for _, e := range entries {
		item := &NoteItem{
			Note: e.Note, Used: e.Used, Tags: NoteTagsOf(e.Note),
			DeletePrompt: NoteDeletePrompt(e.Note, e.Used),
			Status:       &NoteStatus{Note: e.Note, View: NoteStatusList, Suggest: suggest},
			Again:        NoteAgainOf(svc, e.Note, false),
		}

		if e.StackSize > 1 {
			item.Status.Stack = e.StackID
		}

		if suggest {
			s, err := svc.SuggestTags(c, user, e.Note.ID)
			if err != nil {
				return nil, err
			}

			item.Suggestions = s
		}

		if last := len(groups) - 1; last >= 0 && e.StackSize > 1 && groups[last].StackID == e.StackID {
			groups[last].Items = append(groups[last].Items, item)
			continue
		}

		g := &NoteGroup{StackID: e.StackID, Items: []*NoteItem{item}}
		if e.StackSize > 1 {
			g.Stack = true
			g.Label = stackLabel(e.Note.CapturedAt, user, e.StackSize)
			g.Href = NoteQuery{StackID: e.StackID}.URL(NotesPath)
		}

		groups = append(groups, g)
	}

	return groups, nil
}

// Notes is the notes list for the view of the request: the tag bar, the
// notes grouped by stack and the link to the next page.
func Notes(c *gin.Context, svc *notes.Service, userData *auth.UserData) mo.Result[*NotesPage] {
	user := userData.DBUser
	q := ParseNoteQuery(c)

	tags, err := svc.Tags(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	page, err := svc.List(c, user, q.Filter(c.Query("cursor")))
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	groups, err := noteGroups(svc, c, user, page.Entries, q.Inbox)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	focus, err := svc.Focus(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	sources, err := svc.Sources(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	out := &NotesPage{
		BasePage: getBasePage(c, "Notes", userData),
		Query:    q,
		Bar:      NewNoteTagBar(NotesPath, "", true, q, tags),
		Groups:   groups,
		Focus:    focus,
		Views: []NoteLink{
			{Label: "Inbox", Href: q.withInbox(!q.Inbox).URL(NotesPath), Active: q.Inbox},
			{Label: "Unused", Href: q.withUnused(!q.Unused).URL(NotesPath), Active: q.Unused},
		},
	}

	if page.Next != "" {
		more := q.Values()
		more.Set("cursor", page.Next)
		out.More = NotesPath + "?" + more.Encode()
	}

	for _, s := range sources {
		on := s.Source.ID == q.SourceID

		next := s.Source.ID
		if on {
			next = ""
		}

		out.Sources = append(out.Sources, NoteLink{
			Label: s.Source.Title, Count: s.Notes, Active: on, Href: q.withSource(next).URL(NotesPath),
		})
	}

	return mo.Ok(out)
}

// NotesManagePage is the screen that renames and merges tags and sources,
// hides tags and sets the focus.
func NotesManagePage(c *gin.Context, svc *notes.Service, userData *auth.UserData) mo.Result[*NotesPage] {
	user := userData.DBUser

	tags, err := svc.Tags(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	sources, err := svc.Sources(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	focus, err := svc.Focus(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	return mo.Ok(&NotesPage{
		BasePage: getBasePage(c, "Tags and sources", userData),
		Manage:   &NoteManage{Tags: tags, Sources: sources, Focus: focus},
	})
}

// EditNotePage is /controls/notes/:id/edit.
type EditNotePage struct {
	*BasePage
	Note *model.Note
	// Tags is the tags set by hand, as the form shows them.
	Tags         string
	DeletePrompt string
	// Chips are all the tags of the note, the ones of its text included.
	Chips []NoteTag
	// Status is the transcription status the form shows for a recording.
	Status *NoteStatus
	// Again is the "Transcribe again as…" control, nil when the note has none.
	Again *NoteAgain
}

// EditNote loads the note for its edit page; a note that isn't the viewer's
// is not found.
func EditNote(c *gin.Context, svc *notes.Service, userData *auth.UserData, id string) mo.Result[*EditNotePage] {
	note, err := svc.Get(c, userData.DBUser, id)
	if err != nil {
		return mo.Err[*EditNotePage](err)
	}

	links, err := svc.PostLinks(c, userData.DBUser, id)
	if err != nil {
		return mo.Err[*EditNotePage](err)
	}

	return mo.Ok(&EditNotePage{
		BasePage:     getBasePage(c, "Edit note", userData),
		Note:         note,
		Tags:         strings.Join(note.Tags, " "),
		Chips:        NoteTagsOf(note),
		DeletePrompt: NoteDeletePrompt(note, links > 0),
		Status:       &NoteStatus{Note: note, View: NoteStatusEdit, Base: note.Body},
		Again:        NoteAgainOf(svc, note, true),
	})
}

// NotesSummary is the Notes section of the controls page: how many notes wait
// in the Inbox and the newest ones.
type NotesSummary struct {
	// Inbox is how many notes have no tag.
	Inbox  int
	Newest []*model.Note
}

// summaryNewest is how many notes the controls page shows.
const summaryNewest = 5

// ControlsNotesPage is the controls page with its Notes section.
type ControlsNotesPage struct {
	*ControlsPage
	Notes *NotesSummary
}

// ControlsNotes adds the Notes section to the controls page.
func ControlsNotes(c *gin.Context, svc *notes.Service, page *ControlsPage) (*ControlsNotesPage, error) {
	user := page.User.DBUser

	inbox, err := svc.InboxCount(c, user)
	if err != nil {
		return nil, err
	}

	newest, err := svc.List(c, user, notes.Filter{})
	if err != nil {
		return nil, err
	}

	sum := &NotesSummary{Inbox: inbox}

	for _, e := range newest.Entries[:min(summaryNewest, len(newest.Entries))] {
		sum.Newest = append(sum.Newest, e.Note)
	}

	return &ControlsNotesPage{ControlsPage: page, Notes: sum}, nil
}

// NoteCapturePath is the capture page: Dictate, Photo and Type.
const NoteCapturePath = NotesPath + "/new"

// The modes ?start= opens the capture page in.
const (
	CaptureStartVoice = "voice"
	CaptureStartPhoto = "photo"
)

// CaptureSharedParam names the note the share target saved, which the
// capture page shows as the last capture.
const CaptureSharedParam = "shared"

// NoteWorkerPath is the capture page's service worker; its scope is the notes
// pages.
const NoteWorkerPath = NotesPath + "/sw.js"

// The query parameters of the service worker's URL: the caps of a share it
// keeps offline, in bytes, the same as the server's (NOTE_AUDIO_MAX_BYTES,
// MAX_UPLOAD_SIZE per photo, NOTE_CAPTURE_MAX_BYTES in all).
const (
	WorkerParamAudioMax   = "audio_max"
	WorkerParamImageMax   = "image_max"
	WorkerParamCaptureMax = "capture_max"
)

// CapturedNote is a saved note as the capture page's last-capture card shows
// it; the field names are those of the page's queue records.
type CapturedNote struct {
	NoteID       string `json:"noteId"`
	Text         string `json:"text"`
	Photos       int    `json:"photos"`
	Voice        bool   `json:"voice"`
	DeletePrompt string `json:"deletePrompt"`
}

// NoteCapturePage is /controls/notes/new. It holds no list: it only captures.
type NoteCapturePage struct {
	*BasePage
	// Focus is the tag every capture gets, empty without one.
	Focus string
	// Dictate is set when recordings are transcribed; without a
	// speech-to-text provider the page has no record button.
	Dictate bool
	// Start is the mode ?start= asks for, when the page offers it.
	Start string
	// AudioMaxSeconds stops a recording; PhotoMaxDimension is the longest
	// side a photo is scaled down to before it is sent.
	AudioMaxSeconds   int
	PhotoMaxDimension int
	// Shared is the note ?shared= names, as JSON for the page's card, when
	// it is the viewer's; empty otherwise.
	Shared string
	// Worker is the URL the page registers its service worker with, which
	// carries the caps of a share the worker keeps offline.
	Worker string
}

// NoteCapture is the capture page for the viewer.
func NoteCapture(c *gin.Context, svc *notes.Service, userData *auth.UserData) mo.Result[*NoteCapturePage] {
	focus, err := svc.Focus(c, userData.DBUser)
	if err != nil {
		return mo.Err[*NoteCapturePage](err)
	}

	limits := svc.Limits()
	out := &NoteCapturePage{
		BasePage:          getBasePage(c, "New note", userData),
		Focus:             focus,
		Dictate:           svc.TranscriptionEnabled(),
		AudioMaxSeconds:   int(limits.AudioMaxDuration / time.Second),
		PhotoMaxDimension: limits.PhotoMaxDimension,
		Worker: NoteWorkerPath + "?" + url.Values{
			WorkerParamAudioMax:   {strconv.FormatInt(limits.AudioMaxBytes, 10)},
			WorkerParamImageMax:   {strconv.FormatInt(svc.MaxUploadSize(), 10)},
			WorkerParamCaptureMax: {strconv.FormatInt(limits.CaptureMaxBytes, 10)},
		}.Encode(),
	}

	if start := c.Query("start"); start == CaptureStartPhoto || start == CaptureStartVoice && out.Dictate {
		out.Start = start
	}

	shared, err := sharedNote(c, svc, userData)
	if err != nil {
		return mo.Err[*NoteCapturePage](err)
	}

	out.Shared = shared

	return mo.Ok(out)
}

// sharedNote is the card of the note ?shared= names, as JSON, or "" when the
// parameter names no note of the viewer.
func sharedNote(c *gin.Context, svc *notes.Service, userData *auth.UserData) (string, error) {
	id := c.Query(CaptureSharedParam)
	if uuid.Validate(id) != nil {
		return "", nil
	}

	n, err := svc.Get(c, userData.DBUser, id)
	if errors.Is(err, service.ErrNotFound) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	b, err := json.Marshal(CapturedNote{
		NoteID:       n.ID,
		Text:         capture.Unquote(n.Body),
		Photos:       len(n.Images),
		Voice:        n.AudioUploadID != nil,
		DeletePrompt: NoteDeletePrompt(n, false),
	})

	return string(b), err
}
