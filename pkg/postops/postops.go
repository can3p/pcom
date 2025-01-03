package postops

import (
	"cmp"
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
	Post         *Post
	Capabilities *CommentCapabilities
	Level        int64
}

func (c *Comment) String() string {
	return fmt.Sprintf("Comment{id:%s, parent_id: %s, date: %s, username: %s}",
		c.ID, c.ParentCommentID.String, c.CreatedAt.Format(time.ANSIC), c.Author.Username)
}

func CanSeePost(p *core.Post, radius userops.ConnectionRadius) bool {
	switch {
	case radius.IsSameUser():
		fallthrough
	case radius.IsDirect():
		fallthrough
	case radius.IsSecondDegree() && p.VisibilityRadius == core.PostVisibilitySecondDegree:
		fallthrough
	case p.VisibilityRadius == core.PostVisibilityPublic:
		return true
	}

	return false
}

type PostCapabilities struct {
	CanViewComments  bool
	CanLeaveComments bool
	CanEdit          bool
	CanShare         bool
}

func GetPostCapabilities(radius userops.ConnectionRadius) *PostCapabilities {
	return &PostCapabilities{
		// it can be different in the future, e.g. if the author disables
		// new comments at some point
		CanViewComments:  radius.IsDirect() || radius.IsSameUser(),
		CanLeaveComments: radius.IsDirect() || radius.IsSameUser(),
		CanEdit:          radius.IsSameUser(),
		CanShare:         radius.IsSameUser(),
	}
}

type Post struct {
	*core.Post
	Author         *core.User
	Via            []*core.User
	Capabilities   *PostCapabilities
	CommentsNumber int64
	Comments       []*Comment
	Radius         userops.ConnectionRadius
	EditPreview    bool
}

func (p *Post) IsPublished() bool {
	return p.PublishedAt.Valid
}

func (p *Post) PostSubject() string {
	return cmp.Or(p.Subject, "No Subject")
}

func ConstructPost(user *core.User, post *core.Post, radius userops.ConnectionRadius, via []*core.User, editPreview bool) *Post {
	var commentsNum int64

	if (radius.IsDirect() || radius.IsSameUser()) && post.R.PostStat != nil {
		commentsNum = post.R.PostStat.CommentsNumber
	}

	return &Post{
		Author:         post.R.User,
		Post:           post,
		CommentsNumber: commentsNum,
		Capabilities:   GetPostCapabilities(radius),
		EditPreview:    editPreview && radius.IsSameUser(), // only authors can preview their own posts
		Radius:         radius,
		Via:            via,
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
		if idx == len(activeSlice) {
			if len(parkedSlices) == 0 {
				break
			}

			lastIdx := len(parkedSlices) - 1

			activeSlice = parkedSlices[lastIdx]
			idx = parkedIdxes[lastIdx]
			parkedSlices = parkedSlices[:lastIdx]
			parkedIdxes = parkedIdxes[:lastIdx]
			level--
			continue
		}

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
		}
	}

	return out
}
