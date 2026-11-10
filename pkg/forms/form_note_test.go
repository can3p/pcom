package forms_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// A new note needs some text; a saved one reloads the list. An edit saves in
// place; what the service refuses shows on the form, a tag under its field
// and a note of somebody else as not found.
func TestNoteForm(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := notes.New(repo.New(db))
	me := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)

	c, _ := newCtx(t)
	empty := forms.NoteFormNew(svc, me, "")
	empty.Input.Body = "  "
	require.Error(t, empty.Validate(c))
	require.Equal(t, "Write something first.", empty.Errors["body"])

	create := forms.NoteFormNew(svc, me, "")
	create.Input.Body, create.Input.Tags = "jotted", "walk, essay"
	require.NoError(t, create.Validate(c))

	c, w := newCtx(t)
	testutil.Must(create.Save(ctx))(t)(c, create)
	require.Equal(t, "true", w.Header().Get("HX-Refresh"))

	note := testutil.Must(factory.Note(ctx, db, me.ID))(t)
	edit := forms.NoteFormNew(svc, me, note.ID)
	edit.Input.Body, edit.Input.Tags = "", "bad!tag"
	require.NoError(t, edit.Validate(c), "an edit may empty the text")
	testutil.Must(edit.Save(ctx))(t)
	require.Contains(t, edit.Errors, "tags")

	theirs := testutil.Must(factory.Note(ctx, db, other.ID))(t)
	forged := forms.NoteFormNew(svc, me, theirs.ID)
	forged.Input.Body = "mine now"
	testutil.Must(forged.Save(ctx))(t)
	require.Equal(t, "Not found.", forged.FormError)
}

// Adding a source reloads the screen; a rename answers with its row, or
// reloads when it merges into another source; a bad title shows under the
// field.
func TestNoteSourceForm(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := notes.New(repo.New(db))
	me := testutil.Must(factory.User(ctx, db))(t)

	add := forms.NoteSourceFormNew(svc, me, nil)
	add.Input.Title = "Walden"

	c, w := newCtx(t)
	testutil.Must(add.Save(ctx))(t)(c, add)
	require.Equal(t, "true", w.Header().Get("HX-Refresh"))

	cape := testutil.Must(factory.NoteSource(ctx, db, me.ID, factory.WithSourceTitle("Cape Cod")))(t)

	rename := forms.NoteSourceFormNew(svc, me, cape)
	require.Equal(t, "Cape Cod", rename.Input.Title, "a rename starts with the title")

	rename.Input.Title = "Cape Cod, MA"
	c, w = newCtx(t)
	testutil.Must(rename.Save(ctx))(t)(c, rename)
	require.Empty(t, w.Header().Get("HX-Refresh"), "a rename answers with the row")

	rename.Input.Title = " "
	testutil.Must(rename.Save(ctx))(t)
	require.Equal(t, "A source needs a title.", rename.Errors["title"])

	merge := forms.NoteSourceFormNew(svc, me, cape)
	merge.Input.Title = "walden"
	c, w = newCtx(t)
	testutil.Must(merge.Save(ctx))(t)(c, merge)
	require.Equal(t, "true", w.Header().Get("HX-Refresh"), "a merge reloads")
}
