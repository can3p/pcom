package proofread

import (
	"context"
	"errors"
	"strings"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// msgModelFailed is what the actor reads when the model did not answer.
const msgModelFailed = "Claude did not answer. This attempt was not counted."

// begin refuses every call while the feature is off, and an anonymous actor.
func (s *Service) begin(actor *model.User) error {
	if s.model == nil {
		return service.ErrNotFound
	}

	if actor == nil {
		return service.ErrNeedsLogin
	}

	return nil
}

// checkText holds the text to the post form's limits, with its messages.
func (s *Service) checkText(subject, body string) error {
	if err := validation.ValidateMinMax("subject", subject, 0, s.limits.PostSubjectMaxLength); err != nil {
		return service.Invalid("subject", err.Error())
	}

	if err := validation.ValidateMinMax("body", body, 0, s.limits.PostBodyMaxLength); err != nil {
		return service.Invalid("body", err.Error())
	}

	return nil
}

// checkNote holds an author note or a keep reason to NoteMaxLength.
func (s *Service) checkNote(field, label, text string) error {
	if err := validation.ValidateMinMax(label, text, 0, s.limits.NoteMaxLength); err != nil {
		return service.Invalid(field, err.Error())
	}

	return nil
}

// ownPost returns the post if the actor may edit it: an unknown post is
// ErrNotFound, somebody else's ErrForbidden, as the posts service answers.
func (s *Service) ownPost(ctx context.Context, actor *model.User, postID string) (*model.Post, error) {
	if _, err := uuid.Parse(postID); err != nil {
		return nil, service.ErrNotFound
	}

	post, err := s.store.PostForEdit(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if err := s.posts.CheckEdit(ctx, actor, post); err != nil {
		return nil, err
	}

	return post, nil
}

// logger is the logger the context carries, or the service's own.
func (s *Service) logger(ctx context.Context) *zap.Logger {
	if l := logging.FromContext(ctx); l.Core().Enabled(zapcore.ErrorLevel) {
		return l
	}

	return s.log
}

// request is the row recording one call.
func (s *Service) request(actor *model.User, kind model.ProofreadKind, postID *string, choice proofread.ModelChoice, usage proofread.Usage, status model.ProofreadStatus) *model.ProofreadRequest {
	name := usage.Model
	if name == "" {
		name = choice.Name
	}

	return &model.ProofreadRequest{
		UserID:       actor.ID,
		Kind:         kind,
		PostID:       postID,
		Model:        name,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		Status:       status,
		CreatedAt:    s.now().UTC(),
	}
}

// failed records a call the model did not answer, which counts against no
// limit, and tells the actor so.
func (s *Service) failed(ctx context.Context, req *model.ProofreadRequest, callErr error) error {
	s.logger(ctx).Error("proofread call failed", zap.String("kind", string(req.Kind)), zap.String("model", req.Model), zap.Error(callErr))

	if err := s.store.InsertProofreadRequest(ctx, req); err != nil {
		return err
	}

	return service.Invalid("", msgModelFailed)
}

// CheckGrammar asks the model for a grammar check of the actor's text and
// returns the suggestions that apply to it.
func (s *Service) CheckGrammar(ctx context.Context, actor *model.User, req GrammarRequest) (*GrammarResult, error) {
	if err := s.begin(actor); err != nil {
		return nil, err
	}

	if err := s.checkText(req.Subject, req.Body); err != nil {
		return nil, err
	}

	if err := s.checkLimits(ctx, actor, model.ProofreadKindGrammar); err != nil {
		return nil, err
	}

	postID, err := s.grammarPostID(ctx, actor, req.PostID)
	if err != nil {
		return nil, err
	}

	in := proofread.GrammarInput{Model: s.models.Grammar, Subject: req.Subject, Body: req.Body}

	out, usage, err := s.model.Grammar(ctx, in)
	if err != nil {
		return nil, s.failed(ctx, s.request(actor, model.ProofreadKindGrammar, postID, in.Model, usage, model.ProofreadStatusFailed), err)
	}

	kept := proofread.ValidateGrammar(in, out)

	if err := s.store.InsertProofreadRequest(ctx, s.request(actor, model.ProofreadKindGrammar, postID, in.Model, usage, model.ProofreadStatusOK)); err != nil {
		return nil, err
	}

	rem, err := s.remaining(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	res := &GrammarResult{Suggestions: make([]Suggestion, 0, len(kept)), Remaining: rem}
	for _, c := range kept {
		res.Suggestions = append(res.Suggestions, Suggestion{
			Field:       c.Field,
			Kind:        c.Kind,
			Quote:       c.Quote,
			Before:      strings.TrimSuffix(c.Match.Needle, c.Quote),
			Occurrence:  c.Match.Occurrence,
			Replacement: c.Replacement,
			Explanation: c.Explanation,
		})
	}

	return res, nil
}

// grammarPostID is the post a grammar check is recorded with: the edited
// post when it is the actor's, otherwise none. A grammar check needs no post.
func (s *Service) grammarPostID(ctx context.Context, actor *model.User, postID string) (*string, error) {
	if postID == "" {
		return nil, nil
	}

	post, err := s.ownPost(ctx, actor, postID)
	if errors.Is(err, service.ErrNotFound) || errors.Is(err, service.ErrForbidden) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &post.ID, nil
}
