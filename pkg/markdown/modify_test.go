package markdown

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestReplaceImageUrls(t *testing.T) {
	src := `test

![IMG_2693.jpeg](rEplaceme.jpg)

wwww

![IMG_2693.jpeg](KeePME.jpg)`

	expected := `test

![IMG_2693.jpeg](replaced11111111111111111111111111111.jpg)

wwww

![IMG_2693.jpeg](keepme.jpg)`

	res := ReplaceImageUrls(src, map[string]string{
		"replaceme.jpg": "replaced11111111111111111111111111111.jpg",
	}, map[string]struct{}{
		"keepme.jpg": {},
	})

	assert.Equal(t, expected, res)
}
