package connections

import (
	"context"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/samber/lo"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// MediationRequest is a request between two of the user's connections that
// the user may sign or dismiss.
type MediationRequest struct {
	Requester *model.User
	Target    *model.User
	Request   *model.UserConnectionMediationRequest
}

// MediationResult is one mediator's decision on a request.
type MediationResult struct {
	Mediation *model.UserConnectionMediator
	Mediator  *model.User
}

// ConnectionRequest is a request addressed to the user that at least one
// mediator signed.
type ConnectionRequest struct {
	Requester  *model.User
	Request    *model.UserConnectionMediationRequest
	Mediations []*MediationResult
}

// Draft is an unpublished post of the user.
type Draft struct {
	PostID  string
	Subject string
	// Excerpt is the start of the body as plain text, for a draft without a
	// subject; empty when the draft has a subject or no text.
	Excerpt       string
	LastUpdatedAt time.Time
}

// NewPost is a scratch pad of the user that holds text: a new post that is
// not a draft yet.
type NewPost struct {
	// PromptID is the prompt the post answers; empty for the plain pad.
	PromptID string
	Subject  string
	// Excerpt is the start of the body as plain text, for a pad without a
	// subject.
	Excerpt string
	KeptAt  time.Time
}

// Controls is what the controls page shows.
type Controls struct {
	DirectConnections       []*model.User
	SecondDegreeConnections []*model.User
	WhitelistedConnections  []*model.User
	MediationRequests       []*MediationRequest
	ConnectionRequests      []*ConnectionRequest
	Drafts                  []*Draft
	NewPosts                []*NewPost
}

// Controls gathers the actor's connections, whitelist, pending requests and drafts.
func (s *Service) Controls(ctx context.Context, actor *model.User) (*Controls, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	directIDs, secondDegreeIDs, _, err := graph.DirectAndSecondDegree(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

	directUsers, err := s.store.UsersByIDs(ctx, directIDs)
	if err != nil {
		return nil, err
	}

	secondDegreeUsers, err := s.store.UsersByIDs(ctx, secondDegreeIDs)
	if err != nil {
		return nil, err
	}

	whitelisted, err := s.store.WhitelistedUsers(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	toDecide, err := s.store.RequestsToDecide(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	connectionRequests := lo.Map(toDecide, func(req *model.UserConnectionMediationRequest, _ int) *ConnectionRequest {
		return &ConnectionRequest{
			Requester: req.WhoUser,
			Request:   req,
			Mediations: lo.Map(req.MediationUserConnectionMediators, func(m *model.UserConnectionMediator, _ int) *MediationResult {
				return &MediationResult{Mediator: m.User, Mediation: m}
			}),
		}
	})

	toMediate, err := s.store.RequestsToMediate(ctx, directIDs, actor.ID)
	if err != nil {
		return nil, err
	}

	mediationRequests := lo.Map(toMediate, func(req *model.UserConnectionMediationRequest, _ int) *MediationRequest {
		return &MediationRequest{Requester: req.WhoUser, Target: req.TargetUser, Request: req}
	})

	rawDrafts, err := s.store.DraftPosts(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	drafts := lo.Map(rawDrafts, func(d *model.Post, _ int) *Draft {
		draft := &Draft{PostID: d.ID, Subject: lo.FromPtr(d.Subject), LastUpdatedAt: lo.FromPtr(d.UpdatedAt)}
		if draft.Subject == "" {
			draft.Excerpt = excerpt(d.Body, s.excerptLength)
		}

		return draft
	})

	pads, err := s.store.ScratchesWithText(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	newPosts := lo.Map(pads, func(p *model.PostScratch, _ int) *NewPost {
		np := &NewPost{PromptID: lo.FromPtr(p.PromptID), Subject: lo.FromPtr(p.Subject), KeptAt: p.UpdatedAt}
		if np.Subject == "" {
			np.Excerpt = excerpt(p.Body, s.excerptLength)
		}

		return np
	})

	return &Controls{
		DirectConnections:       directUsers,
		SecondDegreeConnections: secondDegreeUsers,
		WhitelistedConnections:  whitelisted,
		MediationRequests:       mediationRequests,
		ConnectionRequests:      connectionRequests,
		Drafts:                  drafts,
		NewPosts:                newPosts,
	}, nil
}

// excerpt is the first n characters of the markdown body as plain text, cut
// on a rune boundary with an ellipsis when there is more.
func excerpt(body string, n int) string {
	src := []byte(body)

	var b strings.Builder

	_ = ast.Walk(goldmark.DefaultParser().Parse(text.NewReader(src)), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch v := node.(type) {
		case *ast.Text:
			if entering {
				b.Write(v.Segment.Value(src))

				if v.SoftLineBreak() || v.HardLineBreak() {
					b.WriteByte(' ')
				}
			}
		case *ast.String:
			if entering {
				b.Write(v.Value)
			}
		case *ast.Paragraph, *ast.Heading, *ast.ListItem:
			if entering {
				b.WriteByte(' ')
			}
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			if entering {
				b.WriteByte(' ')

				lines := v.Lines()
				for i := range lines.Len() {
					seg := lines.At(i)
					b.Write(seg.Value(src))
				}
			}
		}

		return ast.WalkContinue, nil
	})

	runes := []rune(strings.Join(strings.Fields(b.String()), " "))
	if len(runes) <= n {
		return string(runes)
	}

	return string(runes[:n]) + "\u2026"
}
