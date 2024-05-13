package postops

import (
	"fmt"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
)

type CommentCapabilities struct {
	CanRespond bool
}

type Comment struct {
	*core.PostComment
	Author       *core.User
	Capabilities *CommentCapabilities
	Level        int64
}

func (c *Comment) String() string {
	return fmt.Sprintf("Comment{id:%s, parent_id: %s, date: %s, username: %s}",
		c.ID, c.ParentCommentID.String, c.CreatedAt.Format(time.ANSIC), c.Author.Username)
}

type PostCapabilities struct {
	CanViewComments  bool
	CanLeaveComments bool
}

func GetPostCapabilities(radius userops.ConnectionRadius) *PostCapabilities {
	return &PostCapabilities{
		// it can be different in the future, e.g. if the author disables
		// new comments at some point
		CanViewComments:  radius.IsDirect() || radius.IsSameUser(),
		CanLeaveComments: radius.IsDirect() || radius.IsSameUser(),
	}
}

type Post struct {
	*core.Post
	Author         *core.User
	Capabilities   *PostCapabilities
	CommentsNumber int64
	Comments       []*Comment
}

func ConstructPost(post *core.Post, radius userops.ConnectionRadius) *Post {
	var commentsNum int64

	if post.R.PostStat != nil {
		commentsNum = post.R.PostStat.CommentsNumber
	}

	return &Post{
		Author:         post.R.User,
		Post:           post,
		CommentsNumber: commentsNum,
		Capabilities:   GetPostCapabilities(radius),
	}
}

func ConstructComments(comments core.PostCommentSlice, radius userops.ConnectionRadius) []*Comment {
	if len(comments) == 0 {
		return nil
	}

	slices.SortStableFunc(comments, func(left *core.PostComment, right *core.PostComment) int {
		return left.CreatedAt.Compare(right.CreatedAt)
	})

	topLevel := []*Comment{}
	nested := map[string][]*Comment{}

	for _, dbComment := range comments {
		comment := &Comment{
			PostComment: dbComment,
			Author:      dbComment.R.User,
			Capabilities: &CommentCapabilities{
				CanRespond: radius.IsSameUser() || radius.IsDirect(),
			},
			Level: 0,
		}

		if comment.ParentCommentID.String != "" {
			if _, ok := nested[comment.ParentCommentID.String]; !ok {
				nested[comment.ParentCommentID.String] = []*Comment{}
			}

			nested[comment.ParentCommentID.String] = append(nested[comment.ParentCommentID.String], comment)
			continue
		}

		topLevel = append(topLevel, comment)
	}

	out := []*Comment{}

	parkedSlices := [][]*Comment{}
	parkedIdxes := []int{}
	activeSlice := topLevel
	idx := 0
	level := int64(0)

	for {
		comment := activeSlice[idx]
		comment.Level = level
		idx++

		out = append(out, comment)

		if childSlice, ok := nested[comment.ID]; ok {
			parkedSlices = append(parkedSlices, activeSlice)
			parkedIdxes = append(parkedIdxes, idx)
			activeSlice = childSlice
			idx = 0
			level++
		} else if idx == len(activeSlice) {
			if len(parkedSlices) == 0 {
				break
			}

			activeSlice = parkedSlices[idx-1]
			idx = parkedIdxes[idx-1]
			parkedSlices = parkedSlices[:idx-1]
			parkedIdxes = parkedIdxes[:idx-1]
			level--
		}
	}

	return out
}
