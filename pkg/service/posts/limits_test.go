package posts_test

import (
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/stretchr/testify/require"
)

func TestTextLimits(t *testing.T) {
	t.Parallel()

	def := posts.New(repo.New(nil), nil, nil)
	small := posts.New(repo.New(nil), nil, nil, posts.WithTextLimits(posts.TextLimits{
		CommentMaxLength: 5, PostBodyMaxLength: 6, PostSubjectMaxLength: 7, PromptMaxLength: 8,
	}))

	require.NoError(t, def.ValidateCommentBody(strings.Repeat("a", posts.DefaultCommentMaxLength)))
	require.Error(t, def.ValidateCommentBody(strings.Repeat("a", posts.DefaultCommentMaxLength+1)))
	require.Error(t, def.ValidatePrompt(strings.Repeat("a", posts.DefaultPromptMaxLength+1)))
	require.NoError(t, def.ValidatePrompt(strings.Repeat("a", posts.DefaultPromptMaxLength)))

	require.NoError(t, small.ValidateCommentBody("abcde"))
	require.ErrorContains(t, small.ValidateCommentBody("abcdef"), "between 3 and 5")
	require.ErrorContains(t, small.ValidatePrompt("abcdefghi"), "between 3 and 8")

	errs := small.ValidateSave(posts.SaveInput{Subject: "12345678", Body: "1234567"})
	require.Contains(t, errs["subject"], "between 0 and 7")
	require.Contains(t, errs["body"], "between 0 and 6")
}
