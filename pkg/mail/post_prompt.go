package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
)

// PostPromptInput is what the post prompt mail shows: who asks, what they
// ask and where to answer.
type PostPromptInput struct {
	From     string
	PromptID string
	To       string
	Asker    string
	Message  string
	Link     string
}

// Header addresses the mail to the recipient of the prompt, unique per
// prompt.
func (in PostPromptInput) Header() Header {
	return Header{UniqueID: in.PromptID, From: FromPcom(in.From), To: To(in.To)}
}

var postPromptMail = declare("post_prompt", "post_prompt", postPromptSamples)

func postPromptSamples() []Sample[PostPromptInput] {
	sample := func(message string) PostPromptInput {
		return PostPromptInput{
			From: SampleFrom, PromptID: "prompt-1", To: "recipient@example.test", Asker: "alice",
			Message: message, Link: SampleSite.Abs("write", "prompt", "prompt-1"),
		}
	}

	return []Sample[PostPromptInput]{
		{Name: "post_prompt", Input: sample("What is your favorite hobby?")},
		{Name: "post_prompt_markup", Input: sample(`What is <b>bold</b> & "quoted" <script>alert(1)</script>?`)},
	}
}

// PostPrompt formats the notification about a prompt for its recipient. It
// returns nil when there is nobody to notify.
func PostPrompt(site links.Site, from string, asker *model.User, recipient *model.User, postPrompt *model.PostPrompt) *Envelope {
	// we're not sending email notifications to ourselves
	if asker.ID == recipient.ID {
		return nil
	}

	return postPromptMail.MustRender(PostPromptInput{
		From:     from,
		PromptID: postPrompt.ID,
		To:       recipient.Email,
		Asker:    asker.Username,
		Message:  postPrompt.Message,
		Link:     site.Abs("write", "prompt", postPrompt.ID),
	})
}
