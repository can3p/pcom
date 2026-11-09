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
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

// NoteQuery is the view of the notes list a URL describes: the search box's
// query (Q, read by notes.ParseSearch: #tag, -#tag and words) and the source.
// The search box and the post editor's panel build their links from it.
type NoteQuery struct {
	SourceID string
	Q        string
}

// ParseNoteQuery reads a NoteQuery from the request's query string.
func ParseNoteQuery(c *gin.Context) NoteQuery {
	return NoteQuery{
		SourceID: c.Query("source"),
		Q:        strings.TrimSpace(c.Query("q")),
	}
}

// Search is the parsed search box of q.
func (q NoteQuery) Search() notes.Search {
	return notes.ParseSearch(q.Q)
}

// Filter is the notes service's filter for q, from the page after cursor.
func (q NoteQuery) Filter(cursor string) notes.Filter {
	search := q.Search()

	return notes.Filter{
		Tags: search.Tags, NotTags: search.NotTags, Query: search.Query, SourceID: q.SourceID, Cursor: cursor,
	}
}

// Values is the query string of q.
func (q NoteQuery) Values() url.Values {
	v := url.Values{}

	if q.SourceID != "" {
		v.Set("source", q.SourceID)
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

// withSearch and withSource are q with the one filter changed and the other
// kept.
func (q NoteQuery) withSearch(s notes.Search) NoteQuery {
	q.Q = s.String()

	return q
}

func (q NoteQuery) withSource(id string) NoteQuery {
	q.SourceID = id

	return q
}

// Filtered reports whether q narrows the list down at all.
func (q NoteQuery) Filtered() bool {
	return q.SourceID != "" || q.Q != ""
}

// NoteFilter is a part of the search on show above the list, with the link
// to the list without it. Not marks a tag left out; Text the search's words.
type NoteFilter struct {
	Label string
	Not   bool
	Text  bool
	Href  string
}

// NoteTagLink is a tag one tap adds to the search.
type NoteTagLink struct {
	Tag  string
	Href string
}

// NoteSearch is the search box of the notes list (partial--note-search.html):
// the query, its parts as removable chips, and the author's tags, most used
// first, that the search doesn't hold yet.
type NoteSearch struct {
	Path    string
	Query   NoteQuery
	Filters []NoteFilter
	Tags    []NoteTagLink
}

// NewNoteSearch builds the search box for the view q at path, with the
// author's tags.
func NewNoteSearch(path string, q NoteQuery, tags []notes.TagCount) *NoteSearch {
	s := q.Search()
	out := &NoteSearch{Path: path, Query: q}

	for _, t := range s.Tags {
		out.Filters = append(out.Filters, NoteFilter{Label: "#" + t, Href: q.withSearch(s.WithoutTag(t)).URL(path)})
	}

	for _, t := range s.NotTags {
		out.Filters = append(out.Filters, NoteFilter{Label: "#" + t, Not: true, Href: q.withSearch(s.WithoutNotTag(t)).URL(path)})
	}

	if s.Query != "" {
		out.Filters = append(out.Filters, NoteFilter{Label: s.Query, Text: true, Href: q.withSearch(s.WithoutQuery()).URL(path)})
	}

	for _, t := range tags {
		if !slices.Contains(s.Tags, t.Tag) && !slices.Contains(s.NotTags, t.Tag) {
			out.Tags = append(out.Tags, NoteTagLink{Tag: t.Tag, Href: q.withSearch(s.WithTag(t.Tag)).URL(path)})
		}
	}

	return out
}

// NoteTag is a tag of a note; FromText says it comes from a #word of the
// body, so it goes with the text. Href is the list searching for the tag.
type NoteTag struct {
	Tag      string
	FromText bool
	Href     string
}

// NoteItem is a note of the list.
type NoteItem struct {
	Note *model.Note
	Used bool
	Tags []NoteTag
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
// or "" for the status alone. On the edit page, Base is the
// body the page was opened with, which every poll sends back with the text
// being edited; Typed says the two differ, so the author changed the text
// while the recording was transcribed: the form keeps it and Transcript shows
// next to it instead.
type NoteStatus struct {
	Note       *model.Note
	View       string
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

	return v.Encode()
}

// NoteItemOf is the list item of the viewer's note n.
func NoteItemOf(c *gin.Context, svc *notes.Service, user *model.User, n *model.Note) (*NoteItem, error) {
	links, err := svc.PostLinks(c, user, n.ID)
	if err != nil {
		return nil, err
	}

	return noteItem(svc, n, links > 0), nil
}

// noteItem is the list item of n; used says whether it is in a post.
func noteItem(svc *notes.Service, n *model.Note, used bool) *NoteItem {
	return &NoteItem{
		Note: n, Used: used, Tags: NoteTagsOf(n),
		DeletePrompt: NoteDeletePrompt(n, used),
		Status:       &NoteStatus{Note: n, View: NoteStatusList},
		Again:        NoteAgainOf(svc, n, false),
	}
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
	Search *NoteSearch
	Items  []*NoteItem
	// RecentTags are the tags of the newest notes, newest first, which the
	// capture box offers one tap away.
	RecentTags []string
	// More is the link that loads the next page, empty on the last.
	More string
	// Sources are the views by source, of the sources that have notes.
	Sources []NoteLink
	// SourceChoices are all the author's sources, which the capture box's
	// Source offers.
	SourceChoices []notes.SourceCount
	// Manage is set on the screen that renames tags and sources.
	Manage *NoteManage
}

// NoteManage is the tag and source management screen.
type NoteManage struct {
	Tags    []notes.TagCount
	Sources []notes.SourceCount
	// Add is the Add source form.
	Add *SourceForm
}

// SourceForm is the Add source form, or a source's edit form when Source is
// set (partial--note-source-form.html): what was typed, the error under the
// title, and the result next to the button: Saved, or Failure for an error
// that is not the title's.
type SourceForm struct {
	Source  *model.NoteSource
	Title   string
	Error   string
	Failure string
	Saved   bool
}

// SourceRow is a source of the management screen as its row shows it
// (partial--note-source-row.html); Saved shows that a rename just saved.
type SourceRow struct {
	Source *model.NoteSource
	Notes  int
	Saved  bool
}

// NoteSourceRow is the row of the viewer's source id, with its note count.
func NoteSourceRow(c *gin.Context, svc *notes.Service, user *model.User, id string) (*SourceRow, error) {
	if _, err := svc.Source(c, user, id); err != nil {
		return nil, err
	}

	sources, err := svc.Sources(c, user)
	if err != nil {
		return nil, err
	}

	for _, s := range sources {
		if s.Source.ID == id {
			return &SourceRow{Source: s.Source, Notes: s.Notes}, nil
		}
	}

	return nil, service.ErrNotFound
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

// NoteTagsOf lists the tags of a note, the ones set by hand first, each
// linking to the list searching for it.
func NoteTagsOf(n *model.Note) []NoteTag {
	return noteTagsIn(n, NoteQuery{})
}

// noteTagsIn lists the tags of a note as the list of the view q shows them:
// each links to the view with the tag added to its search.
func noteTagsIn(n *model.Note, q NoteQuery) []NoteTag {
	var out []NoteTag

	search := q.Search()
	add := func(tag string, fromText bool) {
		out = append(out, NoteTag{Tag: tag, FromText: fromText, Href: q.withSearch(search.WithTag(tag)).URL(links.Link("notes"))})
	}

	for _, t := range n.Tags {
		add(t, false)
	}

	for _, t := range n.TextTags {
		if !slices.Contains(n.Tags, t) {
			add(t, true)
		}
	}

	return out
}

// recentTags are the tags of the notes, in the order of the notes and then
// of their tags, each once.
func recentTags(entries []notes.Entry) []string {
	var out []string

	for _, e := range entries {
		for _, t := range NoteTagsOf(e.Note) {
			if !slices.Contains(out, t.Tag) {
				out = append(out, t.Tag)
			}
		}
	}

	return out
}

// noteItems are the items of the entries of a list.
func noteItems(svc *notes.Service, entries []notes.Entry) []*NoteItem {
	items := make([]*NoteItem, 0, len(entries))

	for _, e := range entries {
		items = append(items, noteItem(svc, e.Note, e.Used))
	}

	return items
}

// Notes is the notes list for the view of the request: the search box, the
// notes and the link to the next page.
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

	items := noteItems(svc, page.Entries)

	for _, it := range items {
		it.Tags = noteTagsIn(it.Note, q)
	}

	newest := page
	if q.Filtered() || c.Query("cursor") != "" {
		if newest, err = svc.List(c, user, notes.Filter{}); err != nil {
			return mo.Err[*NotesPage](err)
		}
	}

	sources, err := svc.Sources(c, user)
	if err != nil {
		return mo.Err[*NotesPage](err)
	}

	out := &NotesPage{
		BasePage:      getBasePage(c, "Notes", userData),
		Query:         q,
		Search:        NewNoteSearch(links.Link("notes"), q, tags),
		Items:         items,
		RecentTags:    recentTags(newest.Entries),
		SourceChoices: sources,
	}

	if page.Next != "" {
		more := q.Values()
		more.Set("cursor", page.Next)
		out.More = links.Link("notes") + "?" + more.Encode()
	}

	for _, s := range sources {
		on := s.Source.ID == q.SourceID
		if s.Notes == 0 && !on {
			continue
		}

		next := s.Source.ID
		if on {
			next = ""
		}

		out.Sources = append(out.Sources, NoteLink{
			Label: s.Source.Title, Count: s.Notes, Active: on, Href: q.withSource(next).URL(links.Link("notes")),
		})
	}

	return mo.Ok(out)
}

// NotesManagePage is the screen that renames tags and adds and renames
// sources.
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

	return mo.Ok(&NotesPage{
		BasePage: getBasePage(c, "Tags and sources", userData),
		Manage:   &NoteManage{Tags: tags, Sources: sources, Add: &SourceForm{}},
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
	// Sources are the author's sources, which the Source choice offers;
	// SourceID is the note's, "" for none.
	Sources  []notes.SourceCount
	SourceID string
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

	sources, err := svc.Sources(c, userData.DBUser)
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
		Sources:      sources,
		SourceID:     lo.FromPtr(note.SourceID),
	})
}

// NotesSummary is the Notes section of the controls page: the newest notes.
type NotesSummary struct {
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

	newest, err := svc.List(c, user, notes.Filter{})
	if err != nil {
		return nil, err
	}

	sum := &NotesSummary{}

	for _, e := range newest.Entries[:min(summaryNewest, len(newest.Entries))] {
		sum.Newest = append(sum.Newest, e.Note)
	}

	return &ControlsNotesPage{ControlsPage: page, Notes: sum}, nil
}

// The modes ?start= opens the capture page in.
const (
	CaptureStartVoice = "voice"
	CaptureStartPhoto = "photo"
)

// CaptureSharedParam names the note the share target saved, which the
// capture page shows as the last capture.
const CaptureSharedParam = "shared"

// The query parameters of the service worker's URL: the caps of a share it
// keeps offline, in bytes, the same as the server's (NOTE_AUDIO_MAX_BYTES,
// MAX_UPLOAD_SIZE per photo, NOTE_CAPTURE_MAX_BYTES in all), and the URLs it
// works with, which a worker can be handed no other way.
const (
	WorkerParamAudioMax   = "audio_max"
	WorkerParamImageMax   = "image_max"
	WorkerParamCaptureMax = "capture_max"
	WorkerParamPage       = "page"
	WorkerParamUpload     = "upload"
	WorkerParamDelete     = "delete"
	WorkerParamShare      = "share"
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
	limits := svc.Limits()
	out := &NoteCapturePage{
		BasePage:          getBasePage(c, "New note", userData),
		Dictate:           svc.TranscriptionEnabled(),
		AudioMaxSeconds:   int(limits.AudioMaxDuration / time.Second),
		PhotoMaxDimension: limits.PhotoMaxDimension,
		Worker: links.Link("notes_sw") + "?" + url.Values{
			WorkerParamAudioMax:   {strconv.FormatInt(limits.AudioMaxBytes, 10)},
			WorkerParamImageMax:   {strconv.FormatInt(svc.MaxUploadSize(), 10)},
			WorkerParamCaptureMax: {strconv.FormatInt(limits.CaptureMaxBytes, 10)},
			WorkerParamPage:       {links.Link("notes_new")},
			WorkerParamUpload:     {links.Link("action", "note_capture")},
			WorkerParamDelete:     {links.Link("action", "note_delete")},
			WorkerParamShare:      {links.Link("notes_share")},
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
