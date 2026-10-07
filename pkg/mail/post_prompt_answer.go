package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/samber/lo"
)

// PostPromptAnswerInput is what the post prompt answer mail shows: the
// prompt, who answered it and the post they answered with.
type PostPromptAnswerInput struct {
	From      string
	PostID    string
	To        string
	Responder string
	Message   string
	Subject   string
	Link      string
}

// Header addresses the mail to the asker of the prompt, unique per
// answering post.
func (in PostPromptAnswerInput) Header() Header {
	return Header{UniqueID: in.PostID, From: FromPcom(in.From), To: To(in.To)}
}

var postPromptAnswerMail = declare("post_prompt_answer", "post_prompt_answer", postPromptAnswerSamples)

func postPromptAnswerSamples() []Sample[PostPromptAnswerInput] {
	return []Sample[PostPromptAnswerInput]{
		{Name: "post_prompt_answer", Input: PostPromptAnswerInput{
			From: SampleFrom, PostID: "post-2", To: "asker@example.test", Responder: "bob",
			Message: "What is your favorite hobby?", Subject: "My Answer", Link: SampleSite.Abs("post", "post-1"),
		}},
	}
}

// PostPromptAnswer formats the notification for the asker of a prompt that
// the recipient answered with a post.
func PostPromptAnswer(site links.Site, from string, asker, recipient *model.User, post *model.Post, postPrompt *model.PostPrompt) *Envelope {
	return postPromptAnswerMail.MustRender(PostPromptAnswerInput{
		From:      from,
		PostID:    post.ID,
		To:        asker.Email,
		Responder: recipient.Username,
		Message:   postPrompt.Message,
		Subject:   postops.PostSubject(post.Subject),
		Link:      site.Abs("post", lo.FromPtr(postPrompt.PostID)),
	})
}
