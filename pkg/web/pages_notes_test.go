package web_test

import (
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/web"
	"github.com/stretchr/testify/require"
)

func TestNotes_DeletePrompt(t *testing.T) {
	t.Parallel()

	audio := "upload"
	photos := func(n int) []*model.NoteImage { return make([]*model.NoteImage, n) }

	tests := []struct {
		name string
		note model.Note
		used bool
		want string
	}{
		{"plain", model.Note{}, false, "Delete this note? This can't be undone."},
		{"recording", model.Note{RecordingID: &audio}, false, "Delete this note and its recording? This can't be undone."},
		{"photo", model.Note{Images: photos(1)}, false, "Delete this note and 1 photo? This can't be undone."},
		{"recording and photos", model.Note{RecordingID: &audio, Images: photos(2)}, false,
			"Delete this note, its recording and 2 photos? This can't be undone."},
		{"recording, photos and posts", model.Note{RecordingID: &audio, Images: photos(2)}, true,
			"Delete this note, its recording and 2 photos? This can't be undone. The text already inserted into posts stays there."},
		{"used in posts", model.Note{}, true,
			"Delete this note? This can't be undone. The text already inserted into posts stays there."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, web.NoteDeletePrompt(&tt.note, tt.used))
		})
	}
}

func TestNotes_TranscriptOfTheBodyBeforeIt(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Spoken.", web.TranscriptOf("Spoken."))
	require.Equal(t, "Spoken.", web.TranscriptOf("Spoken.", "", " \n"))
	require.Equal(t, "Spoken.", web.TranscriptOf("Typed.\n\nSpoken.", "Typed.\n"))
	require.Equal(t, "Spoken.", web.TranscriptOf("Saved.\n\nSpoken.", "Saved. And more", "Saved."))
	// a body that starts with none of them is shown whole
	require.Equal(t, "Other.\n\nSpoken.", web.TranscriptOf("Other.\n\nSpoken.", "Typed."))
}

func TestNotes_StatusPollSaysWhereTheStatusShows(t *testing.T) {
	t.Parallel()

	require.Empty(t, (&web.NoteStatus{}).Query())
	require.Equal(t, "view=edit", (&web.NoteStatus{View: web.NoteStatusEdit}).Query())
	require.Equal(t, "view=list", (&web.NoteStatus{View: web.NoteStatusList}).Query())
}

func TestNotes_ParseQueryAndValuesRoundTrip(t *testing.T) {
	t.Parallel()

	const raw = "/controls/notes?tag=a&not=c&source=s1&unused=1&inbox=1&q=%20%23a+-%23c+two+words%20"

	c, _ := ginctx.New(t, http.MethodGet, raw, nil)
	q := web.ParseNoteQuery(c)

	require.Equal(t, web.NoteQuery{SourceID: "s1", Q: "#a -#c two words"}, q, "?tag, ?not, ?unused and ?inbox are no filters")
	require.True(t, q.Filtered())
	require.Equal(t, "/controls/notes?q=%23a+-%23c+two+words&source=s1", q.URL(links.Link("notes")))

	c2, _ := ginctx.New(t, http.MethodGet, q.URL(links.Link("notes")), nil)
	require.Equal(t, q, web.ParseNoteQuery(c2))

	require.Equal(t, notes.Filter{
		Tags: []string{"a"}, NotTags: []string{"c"}, SourceID: "s1", Query: "two words", Cursor: "cur",
	}, q.Filter("cur"))

	// nothing is a view that doesn't narrow
	c3, _ := ginctx.New(t, http.MethodGet, "/controls/notes?unused=1&inbox=1&q=+&tag=a", nil)
	empty := web.ParseNoteQuery(c3)
	require.Equal(t, web.NoteQuery{}, empty)
	require.False(t, empty.Filtered())
	require.Empty(t, empty.Values())
	require.Equal(t, links.Link("notes"), empty.URL(links.Link("notes")))
	require.Equal(t, notes.Filter{}, empty.Filter(""))
}

func TestNotes_FilteredByEachFilterAlone(t *testing.T) {
	t.Parallel()

	for name, q := range map[string]web.NoteQuery{
		"source": {SourceID: "s"}, "search": {Q: "x"},
	} {
		require.True(t, q.Filtered(), name)
	}
}

// The search box shows each part of the query as a chip whose link drops it,
// keeping the other filters, and offers the tags the query doesn't hold.
func TestNotes_SearchChips(t *testing.T) {
	t.Parallel()

	tags := []notes.TagCount{{Tag: "essay", Count: 3}, {Tag: "walk", Count: 2}, {Tag: "other", Count: 1}}
	q := web.NoteQuery{SourceID: "s", Q: "#Essay river -#walk bank"}

	s := web.NewNoteSearch("/p", q, tags)
	require.Equal(t, "/p", s.Path)
	require.Equal(t, []web.NoteFilter{
		{Label: "#essay", Href: "/p?q=-%23walk+river+bank&source=s"},
		{Label: "#walk", Not: true, Href: "/p?q=%23essay+river+bank&source=s"},
		{Label: "river bank", Text: true, Href: "/p?q=%23essay+-%23walk&source=s"},
	}, s.Filters)
	require.Equal(t, []web.NoteTagLink{{Tag: "other", Href: "/p?q=%23essay+%23other+-%23walk+river+bank&source=s"}}, s.Tags)

	empty := web.NewNoteSearch("/p", web.NoteQuery{}, tags)
	require.Empty(t, empty.Filters)
	require.Equal(t, []web.NoteTagLink{
		{Tag: "essay", Href: "/p?q=%23essay"}, {Tag: "walk", Href: "/p?q=%23walk"}, {Tag: "other", Href: "/p?q=%23other"},
	}, empty.Tags)
}

func TestNotes_TagsOfANoteHandSetFirst(t *testing.T) {
	t.Parallel()

	n := &model.Note{Tags: []string{"b", "a"}, TextTags: []string{"a", "c"}}
	require.Equal(t, []web.NoteTag{
		{Tag: "b", Href: "/controls/notes?q=%23b"},
		{Tag: "a", Href: "/controls/notes?q=%23a"},
		{Tag: "c", FromText: true, Href: "/controls/notes?q=%23c"},
	}, web.NoteTagsOf(n))
	require.Empty(t, web.NoteTagsOf(&model.Note{}))
}
