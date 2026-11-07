package web_test

import (
	"net/http"
	"testing"

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
		{"recording", model.Note{AudioUploadID: &audio}, false, "Delete this note and its recording? This can't be undone."},
		{"photo", model.Note{Images: photos(1)}, false, "Delete this note and 1 photo? This can't be undone."},
		{"recording and photos", model.Note{AudioUploadID: &audio, Images: photos(2)}, false,
			"Delete this note, its recording and 2 photos? This can't be undone."},
		{"recording, photos and posts", model.Note{AudioUploadID: &audio, Images: photos(2)}, true,
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
	require.Equal(t, "stack=s1&suggest=1&view=list", (&web.NoteStatus{View: web.NoteStatusList, Stack: "s1", Suggest: true}).Query())
}

func TestNotes_QueryCyclesATagThroughIncludeExcludeOff(t *testing.T) {
	t.Parallel()

	q := web.NoteQuery{}.Cycle("essay")
	require.Equal(t, []string{"essay"}, q.Tags)
	require.Equal(t, "/controls/notes?tag=essay", q.URL(web.NotesPath))

	q = q.Cycle("essay")
	require.Empty(t, q.Tags)
	require.Equal(t, []string{"essay"}, q.Not)
	require.Equal(t, "/controls/notes?not=essay", q.URL(web.NotesPath))

	q = q.Cycle("essay")
	require.Equal(t, web.NotesPath, q.URL(web.NotesPath))
}

func TestNotes_ParseQueryAndValuesRoundTrip(t *testing.T) {
	t.Parallel()

	const raw = "/controls/notes?tag=a&tag=b&not=c&hidden=1&source=s1&stack=k1&unused=1&inbox=1&q=%20two+words%20"

	c, _ := ginctx.New(t, http.MethodGet, raw, nil)
	q := web.ParseNoteQuery(c)

	require.Equal(t, web.NoteQuery{
		Tags: []string{"a", "b"}, Not: []string{"c"}, ShowHidden: true, SourceID: "s1", StackID: "k1",
		Unused: true, Inbox: true, Q: "two words",
	}, q)
	require.True(t, q.Filtered())
	require.Equal(t, "/controls/notes?hidden=1&inbox=1&not=c&q=two+words&source=s1&stack=k1&tag=a&tag=b&unused=1", q.URL(web.NotesPath))

	c2, _ := ginctx.New(t, http.MethodGet, q.URL(web.NotesPath), nil)
	require.Equal(t, q, web.ParseNoteQuery(c2))

	require.Equal(t, notes.Filter{
		StackID: "k1", Tags: []string{"a", "b"}, NotTags: []string{"c"}, ShowHidden: true, SourceID: "s1",
		Unused: true, Inbox: true, Query: "two words", Cursor: "cur",
	}, q.Filter("cur"))

	// only "1" turns a flag on; nothing is a view that doesn't narrow
	c3, _ := ginctx.New(t, http.MethodGet, "/controls/notes?hidden=true&unused=0&inbox=yes&q=+", nil)
	empty := web.ParseNoteQuery(c3)
	require.Equal(t, web.NoteQuery{}, empty)
	require.False(t, empty.Filtered())
	require.Empty(t, empty.Values())
	require.Equal(t, web.NotesPath, empty.URL(web.NotesPath))
}

func TestNotes_FilteredByEachFilterAlone(t *testing.T) {
	t.Parallel()

	for name, q := range map[string]web.NoteQuery{
		"tag": {Tags: []string{"a"}}, "not": {Not: []string{"a"}}, "source": {SourceID: "s"}, "stack": {StackID: "k"},
		"unused": {Unused: true}, "inbox": {Inbox: true}, "search": {Q: "x"},
	} {
		require.True(t, q.Filtered(), name)
	}

	// showing hidden notes widens the list, so it is no filter
	require.False(t, web.NoteQuery{ShowHidden: true}.Filtered())
	require.False(t, web.NoteQuery{ShowHidden: true}.WithHidden(false).Values().Has("hidden"))
	require.Equal(t, "1", web.NoteQuery{}.WithHidden(true).Values().Get("hidden"))
}

func TestNotes_TagBarChipsAndHiddenTags(t *testing.T) {
	t.Parallel()

	tags := []notes.TagCount{{Tag: "essay", Count: 3}, {Tag: "walk", Count: 2}, {Tag: "secret", Count: 1, Hidden: true}, {Tag: "other", Count: 4, Hidden: true}}

	bar := web.NewNoteTagBar("/p", "#t", true, web.NoteQuery{Tags: []string{"essay", "other"}, Not: []string{"walk"}}, tags)
	require.Equal(t, 2, bar.HiddenTags)
	require.True(t, bar.Manage)
	require.Equal(t, "#t", bar.Target)
	// a hidden tag is out of the bar unless the view includes it
	require.Equal(t, []web.NoteChip{
		{Tag: "essay", Count: 3, State: "include", Href: "/p?not=walk&not=essay&tag=other"},
		{Tag: "walk", Count: 2, State: "exclude", Href: "/p?tag=essay&tag=other"},
		{Tag: "other", Count: 4, State: "include", Hidden: true, Href: "/p?not=walk&not=other&tag=essay"},
	}, bar.Chips)
	require.Equal(t, "/p?hidden=1&not=walk&tag=essay&tag=other", bar.ShowHiddenHref)

	bar = web.NewNoteTagBar("/p", "", false, web.NoteQuery{ShowHidden: true}, tags)
	require.Len(t, bar.Chips, 4)
	require.Equal(t, "/p", bar.ShowHiddenHref)
}

func TestNotes_TagsOfANoteHandSetFirst(t *testing.T) {
	t.Parallel()

	n := &model.Note{Tags: []string{"b", "a"}, TextTags: []string{"a", "c"}}
	require.Equal(t, []web.NoteTag{{Tag: "b"}, {Tag: "a"}, {Tag: "c", FromText: true}}, web.NoteTagsOf(n))
	require.Empty(t, web.NoteTagsOf(&model.Note{}))
}
