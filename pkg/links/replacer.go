package links

import (
	"strings"

	"github.com/google/uuid"
)

// the whole idea there is to keep only an identifier in the markdown
// source text and give us flexibility to serve the image from
// any source like cdn without touching saved text
func MediaReplacer(inURL string) (bool, string) {
	parts := strings.Split(inURL, ".")

	if len(parts) != 2 {
		return false, ""
	}

	if _, err := uuid.Parse(parts[0]); err != nil {
		return false, ""
	}

	// all the checks are postponed till the actual call
	return true, AbsLink("uploaded_media", inURL)
}
