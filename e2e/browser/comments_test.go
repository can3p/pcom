//go:build browser

package browser_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b3ConnectedPair creates two users who are direct connections of each
// other, the way the app requires before either may comment on the other's
// posts.
func b3ConnectedPair(t testing.TB, app *e2e.App) (*model.User, *model.User) {
	t.Helper()

	ctx := context.Background()

	a := browser.NewUser(t, app)
	b := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, a.ID, b.ID)
	require.NoError(t, err)

	return a, b
}

// A commenter who is a direct connection of the author leaves a top-level
// comment through the real form, submitting it with the commentform
// controller's Ctrl-Enter shortcut rather than clicking the button. The form
// answers with a full page refresh on purpose (the page keeps its scroll
// position), so the new comment shows up after the reload.
func TestComments_LeaveTopLevelComment(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()

	author, friend := b3ConnectedPair(t, app)
	post, err := factory.Post(ctx, app.DB, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(friend))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "No Comments yet"}).Click())

	topForm := page.Locator("#post" + post.ID)
	body := "Hello from the top level comment form"
	textarea := topForm.GetByLabel("Your Comment")
	require.NoError(t, browser.Expect.Locator(textarea).ToBeVisible())
	require.NoError(t, textarea.Fill(body))

	require.NoError(t, textarea.Press("Control+Enter"))

	require.NoError(t, browser.Expect.Locator(page.GetByText("One comment")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.Locator(".post-user-home", playwright.PageLocatorOptions{HasText: body})).ToBeVisible())
}

// Replying to an existing comment nests the reply under it and indents it
// one level; a reply to that reply nests two levels deep. The reply form is
// collapsed until "Reply" opens it, and its Close button collapses
// it again without submitting.
func TestComments_ReplyNestsAndCollapses(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()

	author, friend := b3ConnectedPair(t, app)
	post, err := factory.Post(ctx, app.DB, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))
	require.NoError(t, err)

	top, err := factory.Comment(ctx, app.DB, post.ID, friend.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(author))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	commentCard := page.Locator("#comment" + post.ID + top.ID)
	replyForm := page.Locator("#comment-wrapper" + post.ID + top.ID)
	leaveAComment := commentCard.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Reply", Exact: new(true)})

	require.NoError(t, browser.Expect.Locator(replyForm).ToBeHidden())

	require.NoError(t, leaveAComment.Click())
	require.NoError(t, browser.Expect.Locator(replyForm).ToBeVisible())

	replyTextarea := replyForm.GetByLabel("Your Comment")
	require.NoError(t, browser.Expect.Locator(replyTextarea).ToBeFocused())

	// Close collapses the form again without posting anything.
	require.NoError(t, replyForm.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click())
	require.NoError(t, browser.Expect.Locator(replyForm).ToBeHidden())

	require.NoError(t, leaveAComment.Click())
	require.NoError(t, browser.Expect.Locator(replyForm).ToBeVisible())

	replyBody := "A level one reply"
	require.NoError(t, replyTextarea.Fill(replyBody))
	require.NoError(t, replyForm.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Post a comment"}).Click())

	levelOne := page.Locator(".comment-1")
	require.NoError(t, browser.Expect.Locator(levelOne).ToContainText(replyBody))

	comments, err := factory.ListComments(ctx, app.DB, post.ID)
	require.NoError(t, err)

	var replyComment *model.PostComment
	for _, c := range comments {
		if c.Body == replyBody {
			replyComment = c
		}
	}
	require.NotNil(t, replyComment, "the level one reply was not saved")

	nested, err := factory.Comment(ctx, app.DB, post.ID, friend.ID, factory.ReplyTo(replyComment.ID))
	require.NoError(t, err)

	_, err = page.Reload()
	require.NoError(t, err)

	levelTwo := page.Locator("#comment" + post.ID + nested.ID)
	require.NoError(t, browser.Expect.Locator(levelTwo).ToHaveClass(regexp.MustCompile(`(^|\s)comment-2(\s|$)`)))
}

// A user who isn't a direct connection of the author may see a public post,
// but gets no comment count, no comment thread and no comment form.
func TestComments_ViewerWithoutAccessGetsNoForm(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()

	author := browser.NewUser(t, app)
	stranger := browser.NewUser(t, app)
	post, err := factory.Post(ctx, app.DB, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(stranger))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.Locator(".us-comment-stats")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.GetByLabel("Your Comment")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Post a comment"})).ToHaveCount(0))
}

// Leaving a comment queues a notification email to the post's author and to
// every other participant who has already commented on the post, but never
// to the commenter themselves.
func TestComments_NotifiesAuthorAndParticipants(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()

	author, commenter := b3ConnectedPair(t, app)
	participant := browser.NewUser(t, app)

	post, err := factory.Post(ctx, app.DB, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))
	require.NoError(t, err)

	_, err = factory.Comment(ctx, app.DB, post.ID, participant.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(commenter))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "No Comments yet"}).Click())

	topForm := page.Locator("#post" + post.ID)
	body := "Thanks for writing this"
	require.NoError(t, topForm.GetByLabel("Your Comment").Fill(body))
	require.NoError(t, topForm.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Post a comment"}).Click())

	require.NoError(t, browser.Expect.Locator(page.Locator(".post-user-home", playwright.PageLocatorOptions{HasText: body})).ToBeVisible())

	// a notification of a comment on this post, to anyone
	isNotification := func(m tommy.Mail) bool {
		return strings.HasPrefix(m.Subject, "New comment in") && strings.Contains(m.Text, app.URL+"/posts/"+post.ID)
	}

	emails := app.SentMails(t, "", isNotification)
	require.Len(t, emails, 2)

	var to []string
	for _, e := range emails {
		require.Len(t, e.To, 1)
		to = append(to, e.To[0])
	}

	require.ElementsMatch(t, []string{author.Email, participant.Email}, to)
	require.NotContains(t, to, commenter.Email)
}

// The author fixes a typo in their own comment through the inline Edit form
// and, after the reload, sees the new text and the "edited" marker. A
// connected user sees the marker too but gets no Edit link on that comment.
func TestComments_AuthorEditsOwnComment(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()

	author, friend := b3ConnectedPair(t, app)
	post, err := factory.Post(ctx, app.DB, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityDirectOnly))
	require.NoError(t, err)

	comment, err := factory.Comment(ctx, app.DB, post.ID, friend.ID, factory.WithCommentBody("A comment with a typpo"))
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(friend))

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	card := page.Locator("#comment" + post.ID + comment.ID)
	editForm := page.Locator("#comment-wrapper-edit" + post.ID + comment.ID)
	require.NoError(t, card.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Edit"}).Click())

	textarea := editForm.GetByLabel("Your Comment")
	require.NoError(t, browser.Expect.Locator(textarea).ToHaveValue("A comment with a typpo"))
	require.NoError(t, textarea.Fill("A comment with a typo fixed"))
	require.NoError(t, editForm.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save"}).Click())

	text := card.Locator(".post-user-home")
	require.NoError(t, browser.Expect.Locator(text).ToContainText("A comment with a typo fixed"))
	require.NoError(t, browser.Expect.Locator(text).Not().ToContainText("typpo"))
	require.NoError(t, browser.Expect.Locator(card).ToContainText("edited"))

	other := browser.Page(t, app, browser.As(author))

	_, err = other.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	otherCard := other.Locator("#comment" + post.ID + comment.ID)
	require.NoError(t, browser.Expect.Locator(otherCard).ToContainText("edited"))
	require.NoError(t, browser.Expect.Locator(otherCard.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Edit"})).ToHaveCount(0))

	_, err = other.Goto("/feed")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(other.Locator(".us-feed-comment", playwright.PageLocatorOptions{HasText: "A comment with a typo fixed"})).ToContainText("(edited"))
}
