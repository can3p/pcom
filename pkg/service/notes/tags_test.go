package notes_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/stretchr/testify/require"
)

// Tags come separated by commas or whitespace, in one value or several; what
// a tag may be is checked on save, not here.
func TestSplitTags(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"walk", "#Idea", "book", "read", "two"}, notes.SplitTags(" walk,#Idea ,", "book\tread\n", ",, two"))
	require.Empty(t, notes.SplitTags())
	require.Empty(t, notes.SplitTags(" , "))
}
