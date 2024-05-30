package types

type Replacer[A any] func(in A) (bool, A)

type Link func(name string, args ...string) string

type HTMLView string

const (
	ViewEditPreview HTMLView = "edit_preview"
	ViewSinglePost  HTMLView = "single_post"
	ViewFeed        HTMLView = "feed"
	ViewComment     HTMLView = "comment"
	ViewArticle     HTMLView = "article"
)
