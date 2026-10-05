package postops

import "github.com/can3p/pcom/pkg/model"

// PostPrompt is a prompt to write a post together with the user who asked.
type PostPrompt struct {
	Prompt *model.PostPrompt
	Author *model.User
	Post   *model.Post
}
