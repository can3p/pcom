package mail

import (
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/types"
	"github.com/samber/lo"
)

// PostCommentParticipantsInput is what the mail to a user who commented on a
// post earlier shows about a new comment.
type PostCommentParticipantsInput struct{ postComment }

// Header addresses the mail to the participant.
func (in PostCommentParticipantsInput) Header() Header { return in.header() }

var postCommentParticipantsMail = declare("comment_notification", "post_comment_participants", postCommentParticipantsSamples)

func postCommentParticipantsSamples() []Sample[PostCommentParticipantsInput] {
	return lo.Map(postCommentSamples("post_comment_participants", "participant@example.test", "Great comment!"), func(s postCommentSample, _ int) Sample[PostCommentParticipantsInput] {
		return Sample[PostCommentParticipantsInput]{Name: s.name, Input: PostCommentParticipantsInput{s.in}}
	})
}

// PostCommentParticipants formats the notification about a new comment for a
// user who commented on the post earlier. It returns nil when there is nobody
// to notify. With edited set it is about an edited comment instead.
func PostCommentParticipants(site links.Site, from string, mediaReplacer types.Replacer[string], commentAuthor *model.User, participant *model.User, post *model.Post, comment *model.PostComment, edited bool) (*Envelope, error) {
	// we're not sending email notifications to ourselves
	if commentAuthor.ID == participant.ID {
		return nil, nil
	}

	in, err := newPostComment(site, from, mediaReplacer, commentAuthor, participant, post, comment, edited)
	if err != nil {
		return nil, err
	}

	return postCommentParticipantsMail.Render(PostCommentParticipantsInput{in})
}
