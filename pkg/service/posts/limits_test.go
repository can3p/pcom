package posts

import (
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/stretchr/testify/require"
)

// The service reports its limits for forms and enforces them on save.
func TestTextLimits(t *testing.T) {
	t.Parallel()

	require.Equal(t, DefaultTextLimits, New(repo.New(nil), nil, nil).TextLimits())

	small := New(repo.New(nil), nil, nil, WithTextLimits(TextLimits{CommentMaxLength: 5, PostBodyMaxLength: 6, PostSubjectMaxLength: 7}))
	require.Equal(t, TextLimits{CommentMaxLength: 5, PostBodyMaxLength: 6, PostSubjectMaxLength: 7, PromptMaxLength: DefaultPromptMaxLength},
		small.TextLimits(), "a zero limit keeps its default")

	var invalid *service.ValidationError

	require.NoError(t, small.checkCommentBody("abcde"))
	require.ErrorAs(t, small.checkCommentBody("abcdef"), &invalid)
	require.Equal(t, "body", invalid.Field)
	require.ErrorAs(t, small.checkCommentBody("ab"), &invalid, "shorter than CommentMinLength")

	require.NoError(t, small.checkPrompt("abc"))
	require.ErrorAs(t, New(repo.New(nil), nil, nil, WithTextLimits(TextLimits{PromptMaxLength: 8})).checkPrompt("abcdefghi"), &invalid)
	require.Equal(t, "message", invalid.Field)

	errs := small.checkSave(SaveInput{Subject: "12345678", Body: "1234567", Visibility: "public"})
	require.Contains(t, errs["subject"], "between 0 and 7")
	require.Contains(t, errs["body"], "between 0 and 6")
}
