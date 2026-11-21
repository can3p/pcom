package notes

import "github.com/can3p/pcom/pkg/stt"

// STT is the speech-to-text backend. Its client lives in pkg/stt and is built
// in the composition root; a nil STT turns transcription off.
type STT = stt.Backend
