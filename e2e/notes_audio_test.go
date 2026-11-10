package e2e_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/stretchr/testify/require"
)

// uploadRecording posts data as the recording of a new note and returns the
// response.
func uploadRecording(t *testing.T, app *e2e.App, c *e2e.Client, contentType string, data []byte) *e2e.Response {
	t.Helper()

	var body bytes.Buffer

	mw := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="memo"`)
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, app.URL+"/controls/action/note_audio", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	c.Get("/controls/settings").RequireStatus(http.StatusOK) // learns the CSRF token

	return c.Do(req)
}

// A recording is uploaded and served to its owner only, privately and in
// ranges an <audio> element seeks with; its note's state is a fragment only
// the owner gets.
func TestNotes_AudioIsTheOwners(t *testing.T) {
	t.Parallel()

	// Transcription is on, against a backend nobody answers at: the note
	// waits for its transcription through the whole test.
	app := e2e.Start(t, e2e.WithEnv("STT_PROVIDER", "openai-compat"),
		e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", "http://127.0.0.1:9/stt"), e2e.WithEnv("STT_OPENAI_COMPAT_KEY", "unused"))
	owner := loginAs(t, app, newUser(t, app))
	other := loginAs(t, app, newUser(t, app))
	data := []byte("\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01\x42\x82\x84webm and the rest of a memo")

	uploadRecording(t, app, owner, "image/png", []byte("\x89PNG\r\n\x1a\n")).RequireStatus(http.StatusBadRequest)

	resp := uploadRecording(t, app, owner, "audio/webm;codecs=opus", data).RequireStatus(http.StatusOK)

	var created struct {
		NoteID string `json:"note_id"`
	}

	require.NoError(t, json.Unmarshal([]byte(resp.Body), &created))

	audio := "/controls/notes/" + created.NoteID + "/audio"

	got := owner.Get(audio).RequireStatus(http.StatusOK)
	require.Equal(t, string(data), got.Body)
	require.Equal(t, "audio/webm", got.Header.Get("Content-Type"))
	require.Equal(t, "private", got.Header.Get("Cache-Control"))

	req, err := http.NewRequest(http.MethodGet, app.URL+audio, nil)
	require.NoError(t, err)
	req.Header.Set("Range", "bytes=4-9")

	part := owner.Do(req).RequireStatus(http.StatusPartialContent)
	require.Equal(t, string(data[4:10]), part.Body)
	require.Equal(t, "bytes 4-9/"+strconv.Itoa(len(data)), part.Header.Get("Content-Range"))

	other.Get(audio).RequireStatus(http.StatusNotFound)
	require.True(t, strings.HasPrefix(app.Client(t).Get(audio).RequireStatus(http.StatusFound).Location(), "/login"))

	status := owner.Get("/controls/notes/" + created.NoteID + "/status").RequireStatus(http.StatusOK)
	require.Contains(t, status.Body, "Transcribing…")
	other.Get("/controls/notes/" + created.NoteID + "/status").RequireStatus(http.StatusNotFound)
}
