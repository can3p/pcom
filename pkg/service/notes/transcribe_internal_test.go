package notes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The forms Whisper writes a spoken tag in become #tags; words that only
// look like one stay.
func TestNotes_SpokenHashtags(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"Buy oat milk hashtag groceries":            "Buy oat milk #groceries",
		"Buy oat milk. Hashtag Groceries.":          "Buy oat milk. #groceries.",
		"An idea. Hashtag, essay-on-walking.":       "An idea. #essay-on-walking.",
		"Hash tag errands and hash-tag Home":        "#errands and #home",
		"Купить молоко. Хэштег Покупки. хештег дом": "Купить молоко. #покупки. #дом",
		"Already typed #done":                       "Already typed #done",
		"Hashtags are a thing; so is #hashtag":      "Hashtags are a thing; so is #hashtag",
		"The word hashtag.":                         "The word hashtag.",
	} {
		require.Equal(t, want, spokenHashtags(in), in)
	}
}
