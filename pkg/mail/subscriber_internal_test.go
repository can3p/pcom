package mail

// subscriberSampleInput is the new post mail sent to an email subscriber of
// the author, a test-only sample that proves the subscriber footer and the
// RFC 8058 headers until a mail to subscribers declares samples of its own.
type subscriberSampleInput struct {
	NewPostInput
}

func (in subscriberSampleInput) Header() Header {
	h := in.NewPostInput.Header()
	h.Audience = AudienceSubscriber
	h.AuthorUsername = in.Username
	h.ManageURL = in.Site.Root + "/subscriptions/manage-token-1"
	h.UnsubscribeURL = in.Site.Root + "/subscriptions/unsubscribe-token-1"

	return h
}

func init() {
	register("test_new_post_subscriber", parse("post_notification", "new_post", func() []Sample[subscriberSampleInput] {
		return []Sample[subscriberSampleInput]{
			{Name: "new_post_subscriber", Input: subscriberSampleInput{newPostSamples()[0].Input}},
		}
	}))
}
