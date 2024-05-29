package mdext

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func TestHandleReplacer(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithExtensions(
			NewHandle(),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewUserHandleRenderer(func(in []byte) (bool, []byte) {
					return true, []byte("https://test/" + string(in))
				}), 500),
			),
		),
	)

	in := []byte("this is a test with @johndoe")
	out := `<p>this is a test with <a href="https://test/johndoe" class="user-handle">@johndoe</a></p>`

	var writer bytes.Buffer

	_ = parser.Convert(in, &writer)

	assert.Equal(t, out, strings.TrimSpace(writer.String()))
}
