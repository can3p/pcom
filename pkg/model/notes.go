package model

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

// NoteTranscriptionStatus is the Postgres enum note_transcription_status:
// where the transcription of a note's recording stands. A note without a
// recording is none.
type NoteTranscriptionStatus string

// The values of NoteTranscriptionStatus.
const (
	NoteTranscriptionNone    NoteTranscriptionStatus = "none"
	NoteTranscriptionPending NoteTranscriptionStatus = "pending"
	NoteTranscriptionDone    NoteTranscriptionStatus = "done"
	NoteTranscriptionFailed  NoteTranscriptionStatus = "failed"
)

// NotePostLinkHow is the Postgres enum note_post_link_how: how a note came
// into a post.
type NotePostLinkHow string

// The values of NotePostLinkHow.
const (
	NotePostLinkInserted NotePostLinkHow = "inserted"
)

// STTStatus is how a call to a speech-to-text backend ended. Only ok calls
// count against the daily limits.
type STTStatus string

// The values of STTStatus.
const (
	STTStatusOK     STTStatus = "ok"
	STTStatusFailed STTStatus = "failed"
)

// Note is a row of notes: a user's private note. Tags are set by hand,
// TextTags are the #words of the body.
type Note struct {
	bun.BaseModel `bun:"table:notes"`

	ID                  string                  `bun:"id,pk"`
	UserID              string                  `bun:"user_id"`
	Body                string                  `bun:"body,default:''"`
	Tags                []string                `bun:"tags,array,default:'{}'"`
	SourceID            *string                 `bun:"source_id"`
	RecordingID         *string                 `bun:"recording_id"`
	TextTags            []string                `bun:"text_tags,array,default:'{}'"`
	TranscriptionStatus NoteTranscriptionStatus `bun:"transcription_status,default:'none'"`
	TranscriptionError  *string                 `bun:"transcription_error"`
	CapturedAt          time.Time               `bun:"captured_at,default:now()"`
	CreatedAt           time.Time               `bun:"created_at,default:now()"`
	UpdatedAt           time.Time               `bun:"updated_at,default:now()"`

	User   *User        `bun:"rel:belongs-to,join:user_id=id"`
	Source *NoteSource  `bun:"rel:belongs-to,join:source_id=id"`
	Images []*NoteImage `bun:"rel:has-many,join:id=note_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *Note) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// NoteTag is a tag of a note; FromText says it comes from a #word of the
// body, so it goes with the text.
type NoteTag struct {
	Tag      string
	FromText bool
}

// AllTags lists the note's tags, the ones set by hand first, each once.
func (m *Note) AllTags() []NoteTag {
	out := make([]NoteTag, 0, len(m.Tags)+len(m.TextTags))

	for _, t := range m.Tags {
		out = append(out, NoteTag{Tag: t})
	}

	for _, t := range m.TextTags {
		if !slices.Contains(m.Tags, t) {
			out = append(out, NoteTag{Tag: t, FromText: true})
		}
	}

	return out
}

// NoteSource is a row of note_sources: where notes come from, named by its
// title alone. MatchKey is NoteSourceMatchKey of the title, unique per user.
type NoteSource struct {
	bun.BaseModel `bun:"table:note_sources"`

	ID       string `bun:"id,pk"`
	UserID   string `bun:"user_id"`
	Title    string `bun:"title"`
	MatchKey string `bun:"match_key"`
}

// NoteSourceMatchKey is how a source is found again: its title lowercased with
// its whitespace collapsed, so the same title typed with another case or
// spacing is the same source. It lives with the model, not the notes service,
// so the test factory makes sources the service finds.
func NoteSourceMatchKey(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

// NoteRecording is a row of note_recordings: a note's recording, the object
// Fname (under the notes service's AudioPrefix) in the media bucket, which
// only its owner may fetch. It is not a MediaUpload, so it is never an image.
type NoteRecording struct {
	bun.BaseModel `bun:"table:note_recordings"`

	ID          string    `bun:"id,pk"`
	UserID      string    `bun:"user_id"`
	Fname       string    `bun:"fname"`
	ContentType string    `bun:"content_type"`
	SizeBytes   int64     `bun:"size_bytes"`
	CreatedAt   time.Time `bun:"created_at,default:now()"`
}

// BeforeAppendModel stamps the row's timestamps on insert.
func (m *NoteRecording) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}

// NoteImage is a row of note_images: one photo of a note, in the order of
// Position.
type NoteImage struct {
	bun.BaseModel `bun:"table:note_images"`

	NoteID   string `bun:"note_id,pk"`
	UploadID string `bun:"upload_id,pk"`
	Position int    `bun:"position"`

	Upload *MediaUpload `bun:"rel:belongs-to,join:upload_id=id"`
}

// NotePostLink is a row of note_post_links: a note used in a post.
type NotePostLink struct {
	bun.BaseModel `bun:"table:note_post_links"`

	NoteID    string          `bun:"note_id,pk"`
	PostID    string          `bun:"post_id,pk"`
	How       NotePostLinkHow `bun:"how"`
	CreatedAt time.Time       `bun:"created_at,default:now()"`
}

// BeforeAppendModel stamps the row's timestamps on insert.
func (m *NotePostLink) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}

// STTRequest is a row of stt_requests: one call to a speech-to-text backend.
// NoteID is nil once the note is deleted.
type STTRequest struct {
	bun.BaseModel `bun:"table:stt_requests"`

	ID        string    `bun:"id,pk"`
	UserID    string    `bun:"user_id"`
	NoteID    *string   `bun:"note_id"`
	Provider  string    `bun:"provider"`
	Model     string    `bun:"model"`
	Seconds   int       `bun:"seconds"`
	Status    STTStatus `bun:"status"`
	CreatedAt time.Time `bun:"created_at,default:now()"`
}

// BeforeAppendModel stamps the row's timestamps on insert.
func (m *STTRequest) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}
