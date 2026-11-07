package notes_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

type world struct {
	ctx   context.Context
	db    *sql.DB
	svc   *notes.Service
	queue *fakequeue.Queue
	me    *model.User
	other *model.User
	now   time.Time
}

func newWorld(t *testing.T, opts ...notes.Option) *world {
	t.Helper()

	w := &world{ctx: context.Background(), db: testdb.New(t).DB, queue: fakequeue.New(), now: time.Now().UTC().Truncate(time.Second)}
	w.me = testutil.Must(factory.User(w.ctx, w.db))(t)
	w.other = testutil.Must(factory.User(w.ctx, w.db))(t)
	w.svc = notes.New(repo.New(w.db), append([]notes.Option{notes.WithJobs(w.queue), notes.WithClock(func() time.Time { return w.now })}, opts...)...)

	return w
}

func (w *world) note(t *testing.T, u *model.User, opts ...factory.NoteOpt) *model.Note {
	t.Helper()

	return testutil.Must(factory.Note(w.ctx, w.db, u.ID, opts...))(t)
}

func (w *world) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int

	require.NoError(t, w.db.QueryRowContext(w.ctx, query, args...).Scan(&n))

	return n
}

// ids lists the notes of a page, newest first.
func ids(p *notes.Page) []string {
	out := []string{}
	for _, e := range p.Entries {
		out = append(out, e.Note.ID)
	}

	return out
}

func (w *world) list(t *testing.T, f notes.Filter) []string {
	t.Helper()

	p, err := w.svc.List(w.ctx, w.me, f)
	require.NoError(t, err)

	return ids(p)
}

// Somebody else's note or source is not found, whatever the method.
func TestOtherUsersThingsAreNotFound(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	note := w.note(t, w.other)
	src := testutil.Must(factory.NoteSource(w.ctx, w.db, w.other.ID))(t)
	mine := w.note(t, w.me)
	post := testutil.Must(factory.Post(w.ctx, w.db, w.other.ID))(t)
	body := "x"
	book := notes.SourceInput{Kind: model.NoteSourceBook, Title: "T"}

	calls := map[string]func() error{
		"Get":         func() error { _, err := w.svc.Get(w.ctx, w.me, note.ID); return err },
		"Update":      func() error { _, err := w.svc.Update(w.ctx, w.me, note.ID, notes.Edit{Body: &body}); return err },
		"Delete":      func() error { return w.svc.Delete(w.ctx, w.me, note.ID) },
		"DeleteMany":  func() error { _, err := w.svc.DeleteMany(w.ctx, w.me, []string{mine.ID, note.ID}); return err },
		"TagMany":     func() error { return w.svc.TagMany(w.ctx, w.me, []string{mine.ID, note.ID}, []string{"a"}) },
		"SuggestTags": func() error { _, err := w.svc.SuggestTags(w.ctx, w.me, note.ID); return err },
		"SetSource":   func() error { return w.svc.SetSource(w.ctx, w.me, []string{note.ID}, &book) },
		"MarkUsed":    func() error { return w.svc.MarkUsed(w.ctx, w.me, mine.ID, post.ID) },
		"MarkUsedNote": func() error {
			return w.svc.MarkUsed(w.ctx, w.me, note.ID, testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t).ID)
		},
		"List":         func() error { _, err := w.svc.List(w.ctx, w.me, notes.Filter{SourceID: src.ID}); return err },
		"RenameSource": func() error { return w.svc.RenameSource(w.ctx, w.me, src.ID, book) },
		"MergeSources": func() error { return w.svc.MergeSources(w.ctx, w.me, []string{src.ID}, src.ID) },
	}

	for name, call := range calls {
		require.ErrorIs(t, call(), service.ErrNotFound, name)
	}

	// Nothing of the other user's changed, and the failed bulk calls left mine alone.
	require.Equal(t, 2, w.count(t, "select count(*) from notes"))
	require.Equal(t, 0, w.count(t, "select count(*) from notes where tags <> '{}'"))

	tags, err := w.svc.Tags(w.ctx, w.me)
	require.NoError(t, err)
	require.Empty(t, tags)

	require.ErrorIs(t, w.svc.MergeSources(w.ctx, w.me, []string{src.ID}, testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t).ID), service.ErrNotFound)
}

func TestCreateAndUpdateParseTags(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithLimits(notes.Limits{BodyMaxLength: 80, TagMaxLength: 9, TagsMax: 3}))

	n, err := w.svc.Create(w.ctx, w.me, notes.Input{Body: "# Title, #Walking and #deep-work, see a/b#nope &#39;", Tags: []string{" #Essay ", "essay", "WALKING"}})
	require.NoError(t, err)
	require.Equal(t, []string{"essay", "walking"}, n.Tags)
	require.Equal(t, []string{"walking", "deep-work"}, n.TextTags)
	require.Equal(t, w.now, n.CapturedAt)

	// Text tags follow every save; a tag set by an action stays.
	body := "now #other"
	n, err = w.svc.Update(w.ctx, w.me, n.ID, notes.Edit{Body: &body})
	require.NoError(t, err)
	require.Equal(t, []string{"essay", "walking"}, n.Tags)
	require.Equal(t, []string{"other"}, n.TextTags)

	got, err := w.svc.Get(w.ctx, w.me, n.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"other"}, got.TextTags)

	tooMany, tooLong, ugly := "#a #b #c", "#abcdefghij", "x"
	for name, tc := range map[string]struct {
		edit  notes.Edit
		field string
	}{
		"too long body":       {notes.Edit{Body: new(string(make([]byte, 81)))}, "body"},
		"too long text tag":   {notes.Edit{Body: &tooLong}, "body"},
		"too many tags":       {notes.Edit{Body: &tooMany}, "tags"},
		"too long action tag": {notes.Edit{Tags: &[]string{"abcdefghij"}}, "tags"},
		"not a tag":           {notes.Edit{Body: &ugly, Tags: &[]string{"two words"}}, "tags"},
		"digits only":         {notes.Edit{Tags: &[]string{"2024"}}, "tags"},
	} {
		_, err := w.svc.Update(w.ctx, w.me, n.ID, tc.edit)

		var verr *service.ValidationError

		require.ErrorAs(t, err, &verr, name)
		require.Equal(t, tc.field, verr.Field, name)
	}
}

func TestListFilters(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	base := w.now.Add(-48 * time.Hour)
	at := func(m int) factory.NoteOpt { return factory.CapturedAt(base.Add(time.Duration(m) * time.Minute)) }

	book := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	a := w.note(t, w.me, at(0), factory.WithNoteTags("go"), factory.WithNoteBody("Walking in the Park"))
	b := w.note(t, w.me, at(1), factory.WithNoteTextTags("go", "web"), factory.FromSource(book.ID))
	c := w.note(t, w.me, at(2), factory.WithNoteTags("web", "done"))
	d := w.note(t, w.me, at(200))
	w.note(t, w.other, at(3), factory.WithNoteTags("go"))
	post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)
	testutil.Must(factory.NotePostLink(w.ctx, w.db, a.ID, post.ID))(t)
	require.NoError(t, w.svc.HideTag(w.ctx, w.me, "Done"))

	cases := map[string]struct {
		f    notes.Filter
		want []string
	}{
		"hidden tags are left out":      {notes.Filter{}, []string{d.ID, b.ID, a.ID}},
		"show hidden":                   {notes.Filter{ShowHidden: true}, []string{d.ID, c.ID, b.ID, a.ID}},
		"tags of both columns":          {notes.Filter{Tags: []string{"go"}}, []string{b.ID, a.ID}},
		"all of the tags":               {notes.Filter{Tags: []string{"Go", "web"}}, []string{b.ID}},
		"a hidden tag asked for":        {notes.Filter{Tags: []string{"done"}}, []string{c.ID}},
		"not tags":                      {notes.Filter{NotTags: []string{"web"}}, []string{d.ID, a.ID}},
		"inbox":                         {notes.Filter{Inbox: true}, []string{d.ID}},
		"unused":                        {notes.Filter{Unused: true}, []string{d.ID, b.ID}},
		"source":                        {notes.Filter{SourceID: book.ID}, []string{b.ID}},
		"search ignores case":           {notes.Filter{Query: "walking IN"}, []string{a.ID}},
		"search is not a pattern":       {notes.Filter{Query: "%"}, []string{}},
		"inbox of a tag that has notes": {notes.Filter{Inbox: true, Tags: []string{"go"}}, []string{}},
	}

	for name, tc := range cases {
		require.Equal(t, tc.want, w.list(t, tc.f), name)
	}

	// a, b, c are within the gap of each other, d is a stack of its own; the
	// stack counts notes the filter leaves out.
	p, err := w.svc.List(w.ctx, w.me, notes.Filter{})
	require.NoError(t, err)
	require.Equal(t, d.ID, p.Entries[0].StackID)
	require.Equal(t, 1, p.Entries[0].StackSize)
	require.Equal(t, a.ID, p.Entries[1].StackID)
	require.Equal(t, 3, p.Entries[1].StackSize)
	require.Equal(t, a.ID, p.Entries[2].StackID)
	require.True(t, p.Entries[2].Used)
	require.False(t, p.Entries[1].Used)
	require.Equal(t, book.ID, p.Entries[1].Note.Source.ID)

	tags, err := w.svc.Tags(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, []notes.TagCount{{Tag: "go", Count: 2}, {Tag: "web", Count: 2}, {Tag: "done", Count: 1, Hidden: true}}, tags)
}

func TestPagingIsStableOverEqualCaptureTimes(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithPageSize(2))
	same := factory.CapturedAt(w.now.Add(-time.Hour))

	for range 5 {
		w.note(t, w.me, same)
	}

	var got []string

	cursor := ""

	for pages := 0; ; pages++ {
		require.Less(t, pages, 5)

		p, err := w.svc.List(w.ctx, w.me, notes.Filter{Cursor: cursor})
		require.NoError(t, err)

		got = append(got, ids(p)...)

		if cursor = p.Next; cursor == "" {
			break
		}
	}

	require.Len(t, got, 5)
	require.Len(t, map[string]bool{got[0]: true, got[1]: true, got[2]: true, got[3]: true, got[4]: true}, 5)
	require.IsDecreasing(t, got)

	_, err := w.svc.List(w.ctx, w.me, notes.Filter{Cursor: "nonsense"})
	require.ErrorAs(t, err, new(*service.ValidationError))
}

func TestFocus(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithLimits(notes.Limits{FocusIdle: time.Hour}))

	require.NoError(t, w.svc.SetFocus(w.ctx, w.me, "#Essay"))

	focus, err := w.svc.Focus(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, "essay", focus)

	// A capture applies the focus and renews it.
	w.now = w.now.Add(50 * time.Minute)
	n, err := w.svc.Create(w.ctx, w.me, notes.Input{Body: "x", Tags: []string{"mine"}, ApplyFocus: true})
	require.NoError(t, err)
	require.Equal(t, []string{"mine", "essay"}, n.Tags)

	w.now = w.now.Add(50 * time.Minute)
	focus, _ = w.svc.Focus(w.ctx, w.me)
	require.Equal(t, "essay", focus)

	// Left alone, it expires, is not applied and is not revived by a capture.
	w.now = w.now.Add(2 * time.Hour)
	focus, _ = w.svc.Focus(w.ctx, w.me)
	require.Empty(t, focus)

	n, err = w.svc.Create(w.ctx, w.me, notes.Input{Body: "y", ApplyFocus: true})
	require.NoError(t, err)
	require.Empty(t, n.Tags)
	require.NoError(t, w.svc.TouchFocus(w.ctx, w.me))

	focus, _ = w.svc.Focus(w.ctx, w.me)
	require.Empty(t, focus)

	require.NoError(t, w.svc.SetFocus(w.ctx, w.me, "again"))
	require.NoError(t, w.svc.SetFocus(w.ctx, w.me, ""))

	focus, _ = w.svc.Focus(w.ctx, w.me)
	require.Empty(t, focus)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	audio := testutil.Must(factory.NoteAudio(w.ctx, w.db, w.me.ID))(t)
	photo := testutil.Must(factory.MediaUpload(w.ctx, w.db, w.me.ID))(t)
	post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)
	rec := w.note(t, w.me, factory.WithRecording(audio.ID))
	plain := w.note(t, w.me)
	third := w.note(t, w.me)
	testutil.Must(factory.NoteImage(w.ctx, w.db, rec.ID, photo.ID, 0))(t)
	testutil.Must(factory.NotePostLink(w.ctx, w.db, rec.ID, post.ID))(t)
	testutil.Must(factory.STTRequest(w.ctx, w.db, w.me.ID, rec.ID, 30))(t)

	require.NoError(t, w.svc.Delete(w.ctx, w.me, plain.ID))
	require.Empty(t, w.queue.Inserted(), "no recording, no job")

	require.NoError(t, w.svc.Delete(w.ctx, w.me, rec.ID))
	require.Equal(t, []notes.DeleteAudioArgs{{UploadID: audio.ID}}, fakequeue.Of[notes.DeleteAudioArgs](w.queue))
	require.Equal(t, 0, w.count(t, "select count(*) from note_images"))
	require.Equal(t, 0, w.count(t, "select count(*) from note_post_links"))
	require.Equal(t, 1, w.count(t, "select count(*) from stt_requests where note_id is null and seconds = 30"))
	require.Equal(t, 1, w.count(t, "select count(*) from posts where id = $1", post.ID))

	// Gone already is deleted; a bulk delete skips what is gone.
	require.NoError(t, w.svc.Delete(w.ctx, w.me, rec.ID))

	n, err := w.svc.DeleteMany(w.ctx, w.me, []string{third.ID, third.ID, plain.ID})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 0, w.count(t, "select count(*) from notes"))
	require.ErrorIs(t, w.svc.Delete(w.ctx, nil, third.ID), service.ErrNeedsLogin)
}

func TestTagManagement(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	a := w.note(t, w.me, factory.WithNoteBody("Read #Walks today"), factory.WithNoteTextTags("walks"), factory.WithNoteTags("old"))
	b := w.note(t, w.me, factory.WithNoteTags("walk", "old", "keep"))
	other := w.note(t, w.other, factory.WithNoteTags("old"))

	require.NoError(t, w.svc.HideTag(w.ctx, w.me, "old"))
	require.NoError(t, w.svc.TagMany(w.ctx, w.me, []string{a.ID, b.ID}, []string{"Bulk", "keep"}))
	require.NoError(t, w.svc.RenameTag(w.ctx, w.me, "walks", "walk"))
	require.NoError(t, w.svc.MergeTags(w.ctx, w.me, []string{"old", "bulk"}, "new"))

	a, _ = w.svc.Get(w.ctx, w.me, a.ID)
	b, _ = w.svc.Get(w.ctx, w.me, b.ID)
	require.Equal(t, "Read #walk today", a.Body)
	require.Equal(t, []string{"walk"}, a.TextTags)
	require.Equal(t, []string{"new", "keep"}, a.Tags)
	require.Equal(t, []string{"walk", "new", "keep"}, b.Tags)

	untouched, err := repo.New(w.db).NoteByID(w.ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"old"}, untouched.Tags)

	// The hidden preference moved with the tag.
	tags, err := w.svc.Tags(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, []notes.TagCount{{Tag: "keep", Count: 2}, {Tag: "new", Count: 2, Hidden: true}, {Tag: "walk", Count: 2}}, tags)
	require.Equal(t, 0, w.count(t, "select count(*) from note_tag_prefs where tag = 'old'"))

	require.NoError(t, w.svc.UnhideTag(w.ctx, w.me, "new"))
	require.Equal(t, 0, w.count(t, "select count(*) from note_tag_prefs where hidden"))
}

func TestSuggestTags(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	base := w.now.Add(-72 * time.Hour)
	book := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)

	w.note(t, w.me, factory.CapturedAt(base.Add(-48*time.Hour)), factory.WithNoteTags("deep-work", "unrelated", "walking"))
	w.note(t, w.me, factory.CapturedAt(base.Add(-47*time.Hour)), factory.FromSource(book.ID), factory.WithNoteTags("quotes", "rare"))
	w.note(t, w.me, factory.CapturedAt(base.Add(-46*time.Hour)), factory.WithNoteTags("rare"))
	w.note(t, w.me, factory.CapturedAt(base.Add(time.Minute)), factory.WithNoteTags("session", "walking"))
	target := w.note(t, w.me, factory.CapturedAt(base), factory.FromSource(book.ID), factory.WithNoteBody("some deep work on a Walking day"), factory.WithNoteTags("walking"))

	got, err := w.svc.SuggestTags(w.ctx, w.me, target.ID)
	require.NoError(t, err)
	// The text's tag, then the ones of its source and stack, most carried first; not what it has.
	require.Equal(t, []string{"deep-work", "quotes", "rare"}, got)
}

func TestSources(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	a, b, c := w.note(t, w.me), w.note(t, w.me), w.note(t, w.me)

	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{a.ID}, &notes.SourceInput{Kind: model.NoteSourceBook, Title: "  Walden ", Author: "Henry  Thoreau"}))
	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{b.ID}, &notes.SourceInput{Kind: model.NoteSourceBook, Title: "WALDEN", Author: "henry thoreau"}))
	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{c.ID}, &notes.SourceInput{Kind: model.NoteSourceWeb, Title: "A page", URL: "https://example.test"}))

	srcs, err := w.svc.Sources(w.ctx, w.me)
	require.NoError(t, err)
	require.Len(t, srcs, 2)
	require.Equal(t, "A page", srcs[0].Source.Title)
	require.Equal(t, "Walden", srcs[1].Source.Title)
	require.Equal(t, 2, srcs[1].Notes)

	page, id := srcs[0].Source, srcs[1].Source.ID
	require.ErrorIs(t, w.svc.RenameSource(w.ctx, w.me, page.ID, notes.SourceInput{Kind: model.NoteSourceBook, Title: "walden", Author: "Henry Thoreau"}), service.ErrConflict)
	require.NoError(t, w.svc.RenameSource(w.ctx, w.me, page.ID, notes.SourceInput{Kind: model.NoteSourceWeb, Title: "Renamed"}))

	require.NoError(t, w.svc.MergeSources(w.ctx, w.me, []string{page.ID}, id))
	require.Equal(t, 1, w.count(t, "select count(*) from note_sources"))
	require.Equal(t, 3, w.count(t, "select count(*) from notes where source_id = $1", id))

	// Without notes a source is not listed, and a note can lose its source.
	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{a.ID, b.ID, c.ID}, nil))

	srcs, err = w.svc.Sources(w.ctx, w.me)
	require.NoError(t, err)
	require.Empty(t, srcs)

	err = w.svc.SetSource(w.ctx, w.me, []string{a.ID}, &notes.SourceInput{Kind: model.NoteSourceBook})
	require.ErrorAs(t, err, new(*service.ValidationError))
}

func TestMarkUsed(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	n := w.note(t, w.me)
	post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)

	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, n.ID, post.ID))
	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, n.ID, post.ID))
	require.Equal(t, 1, w.count(t, "select count(*) from note_post_links where how = 'inserted'"))
	require.Empty(t, w.list(t, notes.Filter{Unused: true}))
}
