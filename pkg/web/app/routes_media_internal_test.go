package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// servingMedia serves whatever it is asked for, as the bucket would: a
// recording reaching it would be answered.
type servingMedia struct{ served []string }

func (s *servingMedia) GetImage(context.Context, string, string) (io.Reader, string, error) {
	return bytes.NewReader([]byte("bytes")), "image/webp", nil
}

func (s *servingMedia) ServeImage(_ context.Context, _ server.MediaGetter, _ *http.Request, w http.ResponseWriter, fname string) error {
	s.served = append(s.served, fname)
	_, err := w.Write([]byte("bytes"))

	return err
}

// A note's stored recording is not found on the public media route, though
// the bucket holds it; an image still is served.
func TestNotes_UserMediaRefusesRecordings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	user := testutil.Must(factory.User(ctx, db))(t)
	storage := fakestorage.New()
	svc := notes.New(store, notes.WithJobs(fakequeue.New()), notes.WithMediaStorage(storage))

	note := testutil.Must(svc.CreateFromAudio(ctx, user, strings.NewReader("OggS\x00\x02memo"), "audio/ogg", notes.Input{}))(t)
	recording := testutil.Must(store.MediaUploadByID(ctx, *note.AudioUploadID))(t)
	require.Equal(t, model.MediaKindNoteAudio, recording.Kind)
	require.True(t, testutil.Must(storage.ObjectExists(ctx, recording.UploadedFname))(t))

	media := &servingMedia{}
	handler := serveUserMedia(media, func(n string) string { return "/static/" + n })

	for fname, want := range map[string]int{
		recording.UploadedFname:                    http.StatusNotFound,
		"0190a5c4-7b1e-7d2a-9a43-1c2b3d4e5f60.png": http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/user-media/x/full", nil)
		c.Params = gin.Params{{Key: "fname", Value: fname}, {Key: "class", Value: "full"}}

		handler(c)
		c.Writer.WriteHeaderNow()

		require.Equal(t, want, rec.Code, fname)
	}

	require.Equal(t, []string{"0190a5c4-7b1e-7d2a-9a43-1c2b3d4e5f60.png"}, media.served)
}
