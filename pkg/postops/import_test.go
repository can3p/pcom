package postops

import (
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

func TestExportImport(t *testing.T) {
	md := `This is a test *post*

that spans

few lines`

	initial := &core.Post{
		ID:               uuid.NewString(),
		Subject:          "test subject",
		Body:             md,
		PublishedAt:      null.TimeFrom(time.Now().Round(time.Second)),
		VisibilityRadius: core.PostVisibilityDirectOnly,
	}

	b := SerializePost(initial)

	imported, err := DeserializePost(b)

	assert.NoError(t, err)

	assert.Equal(t, initial, imported)
}
