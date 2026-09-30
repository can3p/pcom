package postops

import "github.com/can3p/pcom/pkg/model/core"

// PostPrompt is a prompt to write a post together with the user who asked.
type PostPrompt struct {
	Prompt *core.PostPrompt
	Author *core.User
	Post   *core.Post
}
