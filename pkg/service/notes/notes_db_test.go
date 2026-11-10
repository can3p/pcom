package notes_test

import (
	"context"
	"database/sql"
	"strings"
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

// get reads a note back from the database.
func (w *world) get(t *testing.T, id string) *model.Note {
	t.Helper()

	return testutil.Must(repo.New(w.db).NoteByID(w.ctx, id))(t)
}

// source reads a source back from the database.
func (w *world) source(t *testing.T, id string) *model.NoteSource {
	t.Helper()

	return testutil.Must(repo.New(w.db).NoteSourceByID(w.ctx, id))(t)
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
	// What the database holds, to compare with afterwards.
	note = testutil.Must(repo.New(w.db).NoteByID(w.ctx, note.ID))(t)
	mySource := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	mine := w.note(t, w.me, factory.FromSource(mySource.ID))
	post := testutil.Must(factory.Post(w.ctx, w.db, w.other.ID))(t)
	body := "x"
	srcID := src.ID

	calls := map[string]func() error{
		"Get":        func() error { _, err := w.svc.Get(w.ctx, w.me, note.ID); return err },
		"Update":     func() error { _, err := w.svc.Update(w.ctx, w.me, note.ID, notes.Edit{Body: &body}); return err },
		"Delete":     func() error { return w.svc.Delete(w.ctx, w.me, note.ID) },
		"DeleteMany": func() error { _, err := w.svc.DeleteMany(w.ctx, w.me, []string{mine.ID, note.ID}); return err },
		"TagMany":    func() error { return w.svc.TagMany(w.ctx, w.me, []string{mine.ID, note.ID}, []string{"a"}) },
		"SetSource":  func() error { return w.svc.SetSource(w.ctx, w.me, []string{note.ID}, "T") },
		"MarkUsed":   func() error { return w.svc.MarkUsed(w.ctx, w.me, mine.ID, post.ID) },
		"MarkUsedNote": func() error {
			return w.svc.MarkUsed(w.ctx, w.me, note.ID, testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t).ID)
		},
		"List":         func() error { _, err := w.svc.List(w.ctx, w.me, notes.Filter{SourceID: src.ID}); return err },
		"RenameSource": func() error { _, err := w.svc.RenameSource(w.ctx, w.me, src.ID, "T"); return err },
		"Source":       func() error { _, err := w.svc.Source(w.ctx, w.me, src.ID); return err },
		// another user's source given to my note, new or edited
		"CreateWithSource": func() error {
			_, err := w.svc.Create(w.ctx, w.me, notes.Input{Body: "x", SourceID: src.ID})
			return err
		},
		"UpdateSource": func() error { _, err := w.svc.Update(w.ctx, w.me, mine.ID, notes.Edit{SourceID: &srcID}); return err },
	}

	for name, call := range calls {
		require.ErrorIs(t, call(), service.ErrNotFound, name)
	}

	// Nothing of the other user's changed, and the failed bulk calls left mine alone.
	require.Equal(t, 2, w.count(t, "select count(*) from notes"))
	require.Equal(t, 0, w.count(t, "select count(*) from notes where tags <> '{}'"))

	store := repo.New(w.db)
	noteAfter, err := store.NoteByID(w.ctx, note.ID)
	require.NoError(t, err)
	require.Equal(t, note.Body, noteAfter.Body)
	require.Equal(t, note.Tags, noteAfter.Tags)
	require.Equal(t, note.TextTags, noteAfter.TextTags)
	require.Equal(t, note.SourceID, noteAfter.SourceID)
	require.True(t, note.UpdatedAt.Equal(noteAfter.UpdatedAt))

	srcAfter, err := store.NoteSourceByID(w.ctx, src.ID)
	require.NoError(t, err)
	require.Equal(t, src.Title, srcAfter.Title)
	require.Equal(t, src.MatchKey, srcAfter.MatchKey)

	tags, err := w.svc.Tags(w.ctx, w.me)
	require.NoError(t, err)
	require.Empty(t, tags)

	// renaming my source to the other user's title is a rename, not a merge
	renamed, err := w.svc.RenameSource(w.ctx, w.me, mySource.ID, src.Title)
	require.NoError(t, err)
	require.Equal(t, mySource.ID, renamed.ID)

	mineAfter, err := store.NoteByID(w.ctx, mine.ID)
	require.NoError(t, err)
	require.Equal(t, &mySource.ID, mineAfter.SourceID)

	srcAfter, err = store.NoteSourceByID(w.ctx, src.ID)
	require.NoError(t, err)
	require.Equal(t, src.Title, srcAfter.Title)
	require.Equal(t, src.MatchKey, srcAfter.MatchKey)
	require.Equal(t, 2, w.count(t, "select count(*) from note_sources"))
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
	b := w.note(t, w.me, at(1), factory.WithNoteTextTags("go", "web", "draft"), factory.FromSource(book.ID), factory.WithNoteBody("walking to work"))
	c := w.note(t, w.me, at(2), factory.WithNoteTags("web", "done"))
	d := w.note(t, w.me, at(200))
	w.note(t, w.other, at(3), factory.WithNoteTags("go"))
	post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)
	testutil.Must(factory.NotePostLink(w.ctx, w.db, a.ID, post.ID))(t)

	search := func(q string) notes.Filter {
		s := notes.ParseSearch(q)
		return notes.Filter{Tags: s.Tags, NotTags: s.NotTags, Query: s.Query}
	}

	cases := map[string]struct {
		f    notes.Filter
		want []string
	}{
		"all":                          {notes.Filter{}, []string{d.ID, c.ID, b.ID, a.ID}},
		"tags of both columns":         {notes.Filter{Tags: []string{"go"}}, []string{b.ID, a.ID}},
		"all of the tags":              {notes.Filter{Tags: []string{"Go", "web"}}, []string{b.ID}},
		"a text-column tag":            {notes.Filter{Tags: []string{"draft"}}, []string{b.ID}},
		"not tags, both columns":       {notes.Filter{NotTags: []string{"web"}}, []string{d.ID, a.ID}},
		"not a tag of the tags column": {notes.Filter{NotTags: []string{"done"}}, []string{d.ID, b.ID, a.ID}},
		"source":                       {notes.Filter{SourceID: book.ID}, []string{b.ID}},
		"search ignores case":          {notes.Filter{Query: "walking IN"}, []string{a.ID}},
		"search is not a pattern":      {notes.Filter{Query: "%"}, []string{}},
		"search box: tag and text":     {search("#GO walking"), []string{b.ID, a.ID}},
		"search box: tag left out":     {search("walking -#web"), []string{a.ID}},
		"search box: tags and text":    {search("#go #draft walking to"), []string{b.ID}},
	}

	for name, tc := range cases {
		require.Equal(t, tc.want, w.list(t, tc.f), name)
	}

	p, err := w.svc.List(w.ctx, w.me, notes.Filter{})
	require.NoError(t, err)
	require.False(t, p.Entries[1].Used)
	require.Equal(t, book.ID, p.Entries[2].Note.Source.ID)
	require.True(t, p.Entries[3].Used)

	tags, err := w.svc.Tags(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, []notes.TagCount{{Tag: "go", Count: 2}, {Tag: "web", Count: 2}, {Tag: "done", Count: 1}, {Tag: "draft", Count: 1}}, tags)
}

func TestPagingIsStableOverEqualCaptureTimes(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithPageSize(2))
	same := factory.CapturedAt(w.now.Add(-time.Hour))

	for range 5 {
		w.note(t, w.me, same)
	}

	var got []string

	cursor, pages := "", 0

	for {
		pages++
		require.LessOrEqual(t, pages, 3)

		p, err := w.svc.List(w.ctx, w.me, notes.Filter{Cursor: cursor})
		require.NoError(t, err)

		if pages == 1 {
			require.Len(t, p.Entries, 2)
			require.NotEmpty(t, p.Next)
		}

		got = append(got, ids(p)...)

		if cursor = p.Next; cursor == "" {
			break
		}
	}

	require.Equal(t, 3, pages)
	require.Len(t, got, 5)
	require.Len(t, map[string]bool{got[0]: true, got[1]: true, got[2]: true, got[3]: true, got[4]: true}, 5)
	require.IsDecreasing(t, got)

	_, err := w.svc.List(w.ctx, w.me, notes.Filter{Cursor: "nonsense"})
	require.ErrorAs(t, err, new(*service.ValidationError))
}

func TestTopTags(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithLimits(notes.Limits{PanelTags: 2}))

	w.note(t, w.me, factory.WithNoteTags("rare"))
	w.note(t, w.me, factory.WithNoteTags("common"), factory.WithNoteTextTags("often"))
	w.note(t, w.me, factory.WithNoteTags("common", "often"))
	w.note(t, w.me, factory.WithNoteTextTags("often"))
	w.note(t, w.other, factory.WithNoteTags("theirs", "theirs2", "theirs3"))

	top, err := w.svc.TopTags(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, []notes.TagCount{{Tag: "often", Count: 3}, {Tag: "common", Count: 2}}, top)

	fresh := newWorld(t)
	none, err := fresh.svc.TopTags(fresh.ctx, fresh.me)
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	audio := testutil.Must(factory.NoteRecording(w.ctx, w.db, w.me.ID))(t)
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
	require.Equal(t, []notes.DeleteAudioArgs{{RecordingID: audio.ID}}, fakequeue.Of[notes.DeleteAudioArgs](w.queue))
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
	a := w.note(t, w.me, factory.WithNoteBody("Read #Walks today #old"), factory.WithNoteTextTags("walks", "old"), factory.WithNoteTags("old", "walks"))
	b := w.note(t, w.me, factory.WithNoteTags("walk", "old", "keep"))
	other := w.note(t, w.other, factory.WithNoteBody("#old"), factory.WithNoteTextTags("old"), factory.WithNoteTags("old"))

	require.NoError(t, w.svc.TagMany(w.ctx, w.me, []string{a.ID, b.ID}, []string{"Bulk", "keep"}))
	require.NoError(t, w.svc.RenameTag(w.ctx, w.me, "walks", "walk"))
	require.NoError(t, w.svc.MergeTags(w.ctx, w.me, []string{"old", "bulk"}, "new"))

	a, err := w.svc.Get(w.ctx, w.me, a.ID)
	require.NoError(t, err)

	b, err = w.svc.Get(w.ctx, w.me, b.ID)
	require.NoError(t, err)

	// Both columns and the body are rewritten.
	require.Equal(t, "Read #walk today #new", a.Body)
	require.Equal(t, []string{"walk", "new"}, a.TextTags)
	require.Equal(t, []string{"new", "walk", "keep"}, a.Tags)
	require.Equal(t, []string{"walk", "new", "keep"}, b.Tags)

	untouched, err := repo.New(w.db).NoteByID(w.ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, "#old", untouched.Body)
	require.Equal(t, []string{"old"}, untouched.Tags)
	require.Equal(t, []string{"old"}, untouched.TextTags)

	tags, err := w.svc.Tags(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, []notes.TagCount{{Tag: "keep", Count: 2}, {Tag: "new", Count: 2}, {Tag: "walk", Count: 2}}, tags)
}

// Sources are titles: the same title whatever its case and spacing is one
// source, added on its own or by giving notes one, listed with or without
// notes; renaming to another source's title merges the two.
func TestSources(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	a, b, c := w.note(t, w.me), w.note(t, w.me), w.note(t, w.me)

	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{a.ID}, "  Walden   Pond "))
	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{b.ID}, "WALDEN pond"))
	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{c.ID}, "A page"))

	empty, err := w.svc.AddSource(w.ctx, w.me, " Cape  Cod ")
	require.NoError(t, err)
	require.Equal(t, "Cape Cod", empty.Title)

	again, err := w.svc.AddSource(w.ctx, w.me, "cape cod")
	require.NoError(t, err)
	require.Equal(t, empty.ID, again.ID, "adding a title that exists finds it")

	srcs, err := w.svc.Sources(w.ctx, w.me)
	require.NoError(t, err)
	require.Len(t, srcs, 3)
	require.Equal(t, []string{"A page", "Cape Cod", "Walden Pond"}, []string{srcs[0].Source.Title, srcs[1].Source.Title, srcs[2].Source.Title})
	require.Equal(t, []int{1, 0, 2}, []int{srcs[0].Notes, srcs[1].Notes, srcs[2].Notes}, "a source without notes is listed")

	page, walden := srcs[0].Source, srcs[2].Source

	got, err := w.svc.Source(w.ctx, w.me, page.ID)
	require.NoError(t, err)
	require.Equal(t, "A page", got.Title)

	// a plain rename, the case of its own title included
	renamed, err := w.svc.RenameSource(w.ctx, w.me, page.ID, " A  Page ")
	require.NoError(t, err)
	require.Equal(t, page.ID, renamed.ID)
	require.Equal(t, "A Page", w.source(t, page.ID).Title)

	// renaming into another source's title merges them
	merged, err := w.svc.RenameSource(w.ctx, w.me, page.ID, "walden pond")
	require.NoError(t, err)
	require.Equal(t, walden.ID, merged.ID)
	require.Equal(t, "Walden Pond", merged.Title)
	require.Equal(t, 2, w.count(t, "select count(*) from note_sources"))
	require.Equal(t, 3, w.count(t, "select count(*) from notes where source_id = $1", walden.ID))

	// an empty title takes the notes' source away; the source stays
	require.NoError(t, w.svc.SetSource(w.ctx, w.me, []string{a.ID, b.ID, c.ID}, " "))
	require.Equal(t, 0, w.count(t, "select count(*) from notes where source_id is not null"))
	require.Equal(t, 2, w.count(t, "select count(*) from note_sources"))

	var verr *service.ValidationError

	long := strings.Repeat("x", notes.DefaultSourceTitleMaxLength+1)

	for name, call := range map[string]func() error{
		"add empty":    func() error { _, err := w.svc.AddSource(w.ctx, w.me, "  "); return err },
		"add long":     func() error { _, err := w.svc.AddSource(w.ctx, w.me, long); return err },
		"rename empty": func() error { _, err := w.svc.RenameSource(w.ctx, w.me, walden.ID, ""); return err },
		"rename long":  func() error { _, err := w.svc.RenameSource(w.ctx, w.me, walden.ID, long); return err },
	} {
		require.ErrorAs(t, call(), &verr, name)
		require.Equal(t, "title", verr.Field, name)
	}

	require.Equal(t, "Walden Pond", w.source(t, walden.ID).Title)
	require.Equal(t, 2, w.count(t, "select count(*) from note_sources"))
}

// A note's source is one of the actor's, chosen by id on create and edit, or
// found or made by title on create; "" takes it away, nil keeps it.
func TestNoteSourceOnCreateAndUpdate(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	walden := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID, factory.WithSourceTitle("Walden")))(t)
	cape := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID, factory.WithSourceTitle("Cape Cod")))(t)

	n, err := w.svc.Create(w.ctx, w.me, notes.Input{Body: "x", SourceID: walden.ID})
	require.NoError(t, err)
	require.Equal(t, walden.ID, *w.get(t, n.ID).SourceID)

	byTitle, err := w.svc.Create(w.ctx, w.me, notes.Input{Body: "y", Source: " walden "})
	require.NoError(t, err)
	require.Equal(t, walden.ID, *byTitle.SourceID, "a title finds the source")

	_, err = w.svc.Update(w.ctx, w.me, n.ID, notes.Edit{SourceID: &cape.ID})
	require.NoError(t, err)
	require.Equal(t, cape.ID, *w.get(t, n.ID).SourceID)

	body := "kept"
	_, err = w.svc.Update(w.ctx, w.me, n.ID, notes.Edit{Body: &body})
	require.NoError(t, err)
	require.Equal(t, cape.ID, *w.get(t, n.ID).SourceID, "an edit without a source keeps it")

	none := ""
	_, err = w.svc.Update(w.ctx, w.me, n.ID, notes.Edit{SourceID: &none})
	require.NoError(t, err)
	require.Nil(t, w.get(t, n.ID).SourceID)

	bad := "not-a-uuid"
	_, err = w.svc.Update(w.ctx, w.me, n.ID, notes.Edit{SourceID: &bad})
	require.ErrorIs(t, err, service.ErrNotFound)

	var verr *service.ValidationError

	_, err = w.svc.Create(w.ctx, w.me, notes.Input{Body: "z", Source: strings.Repeat("x", notes.DefaultSourceTitleMaxLength+1)})
	require.ErrorAs(t, err, &verr)
	require.Equal(t, "source", verr.Field)
	require.Equal(t, 2, w.count(t, "select count(*) from note_sources"))
}

// Every method that takes an actor asks an anonymous one to log in.
func TestNilActorNeedsLogin(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	n := w.note(t, w.me)
	src := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)
	noteIDs, body := []string{n.ID}, "x"

	calls := map[string]func(u *model.User) error{
		"Create":       func(u *model.User) error { _, err := w.svc.Create(w.ctx, u, notes.Input{Body: body}); return err },
		"Update":       func(u *model.User) error { _, err := w.svc.Update(w.ctx, u, n.ID, notes.Edit{Body: &body}); return err },
		"Get":          func(u *model.User) error { _, err := w.svc.Get(w.ctx, u, n.ID); return err },
		"Delete":       func(u *model.User) error { return w.svc.Delete(w.ctx, u, n.ID) },
		"DeleteMany":   func(u *model.User) error { _, err := w.svc.DeleteMany(w.ctx, u, noteIDs); return err },
		"MarkUsed":     func(u *model.User) error { return w.svc.MarkUsed(w.ctx, u, n.ID, post.ID) },
		"List":         func(u *model.User) error { _, err := w.svc.List(w.ctx, u, notes.Filter{}); return err },
		"Tags":         func(u *model.User) error { _, err := w.svc.Tags(w.ctx, u); return err },
		"PostLinks":    func(u *model.User) error { _, err := w.svc.PostLinks(w.ctx, u, n.ID); return err },
		"TagMany":      func(u *model.User) error { return w.svc.TagMany(w.ctx, u, noteIDs, []string{"a"}) },
		"RenameTag":    func(u *model.User) error { return w.svc.RenameTag(w.ctx, u, "a", "b") },
		"MergeTags":    func(u *model.User) error { return w.svc.MergeTags(w.ctx, u, []string{"a"}, "b") },
		"Sources":      func(u *model.User) error { _, err := w.svc.Sources(w.ctx, u); return err },
		"SetSource":    func(u *model.User) error { return w.svc.SetSource(w.ctx, u, noteIDs, "T") },
		"RenameSource": func(u *model.User) error { _, err := w.svc.RenameSource(w.ctx, u, src.ID, "T"); return err },
		"AddSource":    func(u *model.User) error { _, err := w.svc.AddSource(w.ctx, u, "T"); return err },
		"Source":       func(u *model.User) error { _, err := w.svc.Source(w.ctx, u, src.ID); return err },
		"TopTags":      func(u *model.User) error { _, err := w.svc.TopTags(w.ctx, u); return err },
	}

	for name, call := range calls {
		require.ErrorIs(t, call(nil), service.ErrNeedsLogin, name)
	}
}

func TestMarkUsed(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	n := w.note(t, w.me)
	post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)

	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, n.ID, post.ID))
	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, n.ID, post.ID))
	require.Equal(t, 1, w.count(t, "select count(*) from note_post_links where how = 'inserted'"))
}
