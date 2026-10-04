package forms_test

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/stretchr/testify/require"
)

// Each form checks its own fields against the limits its service is
// configured with, not the defaults.
func TestForms_UseConfiguredTextLimits(t *testing.T) {
	t.Parallel()

	postsSvc := posts.New(repo.New(nil), nil, nil, posts.WithTextLimits(posts.TextLimits{
		CommentMaxLength: 5, PostBodyMaxLength: 6, PostSubjectMaxLength: 7, PromptMaxLength: 8,
	}))
	accountsSvc := accounts.New(repo.New(nil), nil, nil,
		accounts.WithProfileAboutMaxLength(4), accounts.WithUserStylesMaxLength(9))

	editComment := func(body string) string {
		f := forms.EditCommentFormNew(postsSvc, nil, "").(*forms.EditCommentForm)
		f.Input.Body = body
		_ = f.Validate(nil)

		return f.Errors["body"]
	}
	post := func(subject, body string) map[string]string {
		f, err := forms.NewPostFormNew(context.Background(), postsSvc, nil, "")
		require.NoError(t, err)

		f.Input.Subject, f.Input.Body, f.Input.Visibility = subject, body, "public"
		_ = f.Validate(nil)

		return f.Errors
	}
	prompt := func(message string) error {
		f := forms.PostPromptFormNew(postsSvc, nil, nil).(*forms.PostPromptForm)
		f.Input.Message = message

		return f.Validate(nil)
	}
	styles := func(css string) string {
		f := forms.SettingsUserStylesNew(accountsSvc, nil)
		f.Input.Styles = css
		_ = f.Validate(nil)

		return f.Errors["styles"]
	}
	about := func(text string) string {
		f := forms.SettingsProfileNew(accountsSvc, nil)
		f.Input.About = text
		_ = f.Validate(nil)

		return f.Errors["about"]
	}

	require.Empty(t, editComment("abcde"))
	require.Contains(t, editComment("abcdef"), "between 3 and 5")
	require.Contains(t, editComment("ab"), "between 3 and 5")

	require.Empty(t, post("1234567", "123456"))
	require.Equal(t, []string{"subject"}, keys(post("12345678", "123456")))
	require.Contains(t, post("12345678", "123456")["subject"], "between 0 and 7")
	require.Equal(t, []string{"body"}, keys(post("1234567", "1234567")))
	require.Contains(t, post("1234567", "1234567")["body"], "between 0 and 6")

	require.ErrorContains(t, prompt("abcdefghi"), "between 3 and 8")

	require.Empty(t, styles("a{b:c;de}"))
	require.Contains(t, styles("a{b:c;def}"), "between 0 and 9")

	require.Empty(t, about("abcd"))
	require.Contains(t, about("abcde"), "at most 4")
}

func keys(m map[string]string) []string {
	return slices.Sorted(maps.Keys(m))
}
