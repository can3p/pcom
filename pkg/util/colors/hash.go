package colors

import (
	"crypto/sha1"
	"fmt"

	"github.com/samber/lo"
)

func toColor(b []byte) string {
	return fmt.Sprintf("#%x", b)
}

func Hash(v string) (string, string) {
	hash := sha1.Sum([]byte(v))
	shaBg := hash[17:]
	shaText := lo.Map(shaBg, func(b byte, idx int) byte { return ^b })
	return toColor(shaBg), toColor(shaText)
}
