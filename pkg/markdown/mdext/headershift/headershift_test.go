package headershift

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/yuin/goldmark"
)

func TestHeaderShift(t *testing.T) {
	parser := goldmark.New(
		goldmark.WithExtensions(NewHeaderShiftExtender(1)),
	)

	examples := []struct {
		in  []byte
		out string
	}{
		{
			in: []byte(`
# header

## subheader`),
			out: `<h2>header</h2>
<h3>subheader</h3>`,
		},
	}

	for idx, ex := range examples {
		in := ex.in
		out := ex.out

		var writer bytes.Buffer

		_ = parser.Convert(in, &writer)

		assert.Equal(t, out, strings.TrimSpace(writer.String()), "[ex %d]:", idx+1)
	}
}
