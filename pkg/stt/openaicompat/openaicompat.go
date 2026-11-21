// Package openaicompat is the speech-to-text backend for services speaking
// OpenAI's /audio/transcriptions API: Groq, OpenAI and the self-hosted
// servers that copy them. It makes the HTTP call and nothing else; wrap it
// in stt.Retry.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/stt"
)

// Name is the backend's provider name, as STT_PROVIDER spells it.
const Name = "openai-compat"

// DefaultTimeout bounds one request when Options sets none.
const DefaultTimeout = 2 * time.Minute

// errorBodyMax is how much of a failed answer an error keeps.
const errorBodyMax = 300

// Options configure the backend.
type Options struct {
	// Endpoint is the API's base URL, without /audio/transcriptions.
	Endpoint string
	Model    string
	Key      string
	// Timeout bounds one request; zero is DefaultTimeout.
	Timeout time.Duration
}

// Backend calls the API. Build it with New.
type Backend struct {
	opts   Options
	client *http.Client
}

var _ stt.Backend = (*Backend)(nil)

// New builds the backend.
func New(opts Options) *Backend {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}

	opts.Endpoint = strings.TrimRight(opts.Endpoint, "/")

	return &Backend{opts: opts, client: &http.Client{}}
}

// Name is the provider name.
func (b *Backend) Name() string { return Name }

// Model is the model the backend asks for.
func (b *Backend) Model() string { return b.opts.Model }

// extensions are the file extensions the API recognizes the format by, which
// it reads from the file name rather than the part's content type.
var extensions = map[string]string{
	"audio/webm":  ".webm",
	"video/webm":  ".webm",
	"audio/mp4":   ".m4a",
	"audio/x-m4a": ".m4a",
	"audio/m4a":   ".m4a",
	"audio/aac":   ".m4a",
	"video/mp4":   ".mp4",
	"audio/ogg":   ".ogg",
	"audio/mpeg":  ".mp3",
	"audio/mp3":   ".mp3",
	"audio/wav":   ".wav",
	"audio/x-wav": ".wav",
	"audio/wave":  ".wav",
	"audio/flac":  ".flac",
}

func filename(contentType string) string {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		media = strings.ToLower(strings.TrimSpace(contentType))
	}

	return "audio" + extensions[media]
}

// answer is the part of a verbose_json answer the backend reads.
type answer struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
}

// Transcribe sends the recording and reads the text, language and duration.
// The language is sent only when audio forces one. A failed status is a
// *stt.StatusError.
func (b *Backend) Transcribe(ctx context.Context, audio stt.Audio) (stt.Result, error) {
	var body bytes.Buffer

	form := multipart.NewWriter(&body)

	fields := [][2]string{{"model", b.opts.Model}, {"response_format", "verbose_json"}}
	if audio.Language != "" {
		fields = append(fields, [2]string{"language", audio.Language})
	}

	for _, f := range fields {
		if err := form.WriteField(f[0], f[1]); err != nil {
			return stt.Result{}, err
		}
	}

	file, err := form.CreateFormFile("file", filename(audio.ContentType))
	if err != nil {
		return stt.Result{}, err
	}

	if _, err := file.Write(audio.Bytes); err != nil {
		return stt.Result{}, err
	}

	if err := form.Close(); err != nil {
		return stt.Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, b.opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.opts.Endpoint+"/audio/transcriptions", &body)
	if err != nil {
		return stt.Result{}, err
	}

	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+b.opts.Key)

	resp, err := b.client.Do(req)
	if err != nil {
		return stt.Result{}, fmt.Errorf("openaicompat: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyMax))

		return stt.Result{}, &stt.StatusError{
			Status:     resp.StatusCode,
			RetryAfter: stt.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
			Message:    strings.TrimSpace(string(msg)),
		}
	}

	var a answer
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return stt.Result{}, fmt.Errorf("openaicompat: unusable answer: %w", err)
	}

	return stt.Result{Text: strings.TrimSpace(a.Text), Language: a.Language, Seconds: a.Duration}, nil
}
