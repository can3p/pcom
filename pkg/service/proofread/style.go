package proofread

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// What the actor reads when a style call is refused.
const (
	msgUnsaved    = "Style review is available once the draft is saved."
	msgNotLatest  = "Only the notes of the latest review can be changed."
	msgNoteStatus = "A note can only be marked done, kept as is, or open."
)

// ReviewStyle asks the model for a style review of the actor's post and
// stores it with its notes. The open notes of the previous review become
// carried when a new note continues them and done otherwise.
func (s *Service) ReviewStyle(ctx context.Context, actor *model.User, req StyleRequest) (*StyleReview, error) {
	if err := s.begin(actor); err != nil {
		return nil, err
	}

	if req.PostID == "" {
		return nil, service.Invalid("postId", msgUnsaved)
	}

	post, err := s.ownPost(ctx, actor, req.PostID)
	if err != nil {
		return nil, err
	}

	if err := s.checkText(req.Subject, req.Body); err != nil {
		return nil, err
	}

	if err := s.checkNote("authorNote", "note", req.AuthorNote); err != nil {
		return nil, err
	}

	if err := s.checkLimits(ctx, actor, model.ProofreadKindStyle); err != nil {
		return nil, err
	}

	reviews, err := s.store.StyleReviews(ctx, post.ID)
	if err != nil {
		return nil, err
	}

	in := proofread.StyleInput{
		Model:      s.models.Style,
		Subject:    req.Subject,
		Body:       req.Body,
		AuthorNote: req.AuthorNote,
		Previous:   previous(reviews),
	}

	out, usage, err := s.model.Style(ctx, in)
	if err != nil {
		return nil, s.failed(ctx, s.request(actor, model.ProofreadKindStyle, &post.ID, in.Model, usage, model.ProofreadStatusFailed), err)
	}

	review := s.newReview(post.ID, reviews, in, out.Summary, proofread.ValidateStyle(in, out))

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.InsertProofreadRequest(ctx, s.request(actor, model.ProofreadKindStyle, &post.ID, in.Model, usage, model.ProofreadStatusOK)); err != nil {
			return err
		}

		if err := tx.InsertStyleReview(ctx, review); err != nil {
			return err
		}

		if len(reviews) == 0 {
			return nil
		}

		carried, done := settle(reviews[0], review)
		if err := tx.SetOpenStyleNotesStatus(ctx, carried, model.StyleNoteStatusCarried); err != nil {
			return err
		}

		return tx.SetOpenStyleNotesStatus(ctx, done, model.StyleNoteStatusDone)
	})
	if err != nil {
		return nil, err
	}

	rem, err := s.remaining(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	return &StyleReview{Review: present(append([]*model.StyleReview{review}, reviews...)), Remaining: rem}, nil
}

// previous is the latest review as the next one is told about it, or nil.
func previous(reviews []*model.StyleReview) *proofread.PreviousReview {
	if len(reviews) == 0 {
		return nil
	}

	r := reviews[0]
	p := &proofread.PreviousReview{
		Number:     r.Number,
		Subject:    r.Subject,
		Body:       r.Body,
		Summary:    r.Summary,
		AuthorNote: r.AuthorNote,
	}

	for _, n := range r.Notes {
		p.Notes = append(p.Notes, proofread.PreviousNote{
			ID:         n.ID,
			Field:      proofread.Field(n.Field),
			Kind:       proofread.NoteKind(n.Kind),
			Quote:      n.Quote,
			Text:       n.Text,
			Status:     proofread.NoteStatus(n.Status),
			KeepReason: n.KeepReason,
		})
	}

	return p
}

// newReview is the review to store: the next number of the post, the text
// it saw and the notes that passed validation, all open.
func (s *Service) newReview(postID string, reviews []*model.StyleReview, in proofread.StyleInput, summary string, notes []proofread.CheckedNote) *model.StyleReview {
	now := s.now().UTC()
	review := &model.StyleReview{
		PostID:     postID,
		Number:     1,
		Subject:    in.Subject,
		Body:       in.Body,
		Summary:    summary,
		AuthorNote: in.AuthorNote,
		CreatedAt:  now,
	}

	if len(reviews) > 0 {
		review.Number = reviews[0].Number + 1
	}

	for i, n := range notes {
		note := &model.StyleReviewNote{
			Position:  i,
			Field:     string(n.Field),
			Kind:      string(n.Kind),
			Quote:     n.Quote,
			Before:    n.Before,
			Text:      n.Text,
			Status:    model.StyleNoteStatusOpen,
			UpdatedAt: now,
		}

		if n.Continues != "" {
			note.ContinuesID = &n.Continues
		}

		review.Notes = append(review.Notes, note)
	}

	return review
}

// settle splits the open notes of the previous review into those a note of
// the new review continues and the rest.
func settle(prev, next *model.StyleReview) (carried, done []string) {
	var continued []string

	for _, n := range next.Notes {
		if n.ContinuesID != nil {
			continued = append(continued, *n.ContinuesID)
		}
	}

	for _, n := range prev.Notes {
		switch {
		case n.Status != model.StyleNoteStatusOpen:
		case slices.Contains(continued, n.ID):
			carried = append(carried, n.ID)
		default:
			done = append(done, n.ID)
		}
	}

	return carried, done
}

// present is the latest of reviews (newest first) as the browser gets it.
// Each note is located again in the text the review saw, which reproduces
// the occurrence it was validated with.
func present(reviews []*model.StyleReview) *Review {
	if len(reviews) == 0 {
		return nil
	}

	latest := reviews[0]
	since := openSince(reviews)

	in := proofread.StyleInput{Subject: latest.Subject, Body: latest.Body}
	out := proofread.StyleOutput{}

	for _, n := range latest.Notes {
		out.Notes = append(out.Notes, proofread.Note{Field: proofread.Field(n.Field), Quote: n.Quote, Before: n.Before})
	}

	checked := proofread.ValidateStyle(in, out)

	r := &Review{
		ID:         latest.ID,
		Number:     latest.Number,
		Summary:    latest.Summary,
		AuthorNote: latest.AuthorNote,
		CreatedAt:  latest.CreatedAt,
		Notes:      make([]Note, 0, len(latest.Notes)),
	}

	for i, n := range latest.Notes {
		note := Note{
			ID:         n.ID,
			Field:      proofread.Field(n.Field),
			Kind:       proofread.NoteKind(n.Kind),
			Quote:      n.Quote,
			Before:     n.Before,
			Text:       n.Text,
			Status:     n.Status,
			KeepReason: n.KeepReason,
		}

		if len(checked) == len(latest.Notes) {
			note.Before = strings.TrimSuffix(checked[i].Match.Needle, n.Quote)
			note.Occurrence = checked[i].Match.Occurrence
		}

		if n.ContinuesID != nil {
			note.ContinuesNumber = since[*n.ContinuesID]
		}

		r.Notes = append(r.Notes, note)
	}

	return r
}

// openSince maps every note of reviews (newest first) to the number of the
// review that first raised it, following the notes it continues.
func openSince(reviews []*model.StyleReview) map[string]int {
	since := map[string]int{}

	for _, r := range slices.Backward(reviews) {
		for _, n := range r.Notes {
			since[n.ID] = r.Number
			if n.ContinuesID != nil {
				if first, ok := since[*n.ContinuesID]; ok {
					since[n.ID] = first
				}
			}
		}
	}

	return since
}

// StyleState returns the style reviews of the actor's post.
func (s *Service) StyleState(ctx context.Context, actor *model.User, postID string) (*StyleState, error) {
	if err := s.begin(actor); err != nil {
		return nil, err
	}

	post, err := s.ownPost(ctx, actor, postID)
	if err != nil {
		return nil, err
	}

	reviews, err := s.store.StyleReviews(ctx, post.ID)
	if err != nil {
		return nil, err
	}

	rem, err := s.remaining(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	st := &StyleState{Review: present(reviews), Earlier: []EarlierReview{}, Remaining: rem}

	if len(reviews) > 1 {
		for _, r := range reviews[1:] {
			st.Earlier = append(st.Earlier, EarlierReview{Number: r.Number, Summary: r.Summary, CreatedAt: r.CreatedAt})
		}
	}

	return st, nil
}

// SetNoteStatus marks a note of the actor's latest review done, kept (with
// an optional reason) or open again. Only a kept note keeps its reason.
func (s *Service) SetNoteStatus(ctx context.Context, actor *model.User, noteID string, status model.StyleNoteStatus, reason string) error {
	if err := s.begin(actor); err != nil {
		return err
	}

	switch status {
	case model.StyleNoteStatusOpen, model.StyleNoteStatusDone:
		reason = ""
	case model.StyleNoteStatusKept:
		reason = strings.TrimSpace(reason)
	default:
		return service.Invalid("status", msgNoteStatus)
	}

	if err := s.checkNote("reason", "reason", reason); err != nil {
		return err
	}

	if _, err := uuid.Parse(noteID); err != nil {
		return service.ErrNotFound
	}

	note, err := s.store.StyleReviewNote(ctx, noteID)
	if errors.Is(err, repo.ErrNotFound) {
		return service.ErrNotFound
	} else if err != nil {
		return err
	}

	post, err := s.ownPost(ctx, actor, note.Review.PostID)
	if err != nil {
		return err
	}

	reviews, err := s.store.StyleReviews(ctx, post.ID)
	if err != nil {
		return err
	}

	if len(reviews) == 0 || reviews[0].ID != note.ReviewID {
		return service.Invalid("", msgNotLatest)
	}

	return s.store.SetStyleNoteStatus(ctx, note.ID, status, reason)
}

// ResetStyle deletes every style review of the actor's post, so the next
// review starts fresh.
func (s *Service) ResetStyle(ctx context.Context, actor *model.User, postID string) error {
	if err := s.begin(actor); err != nil {
		return err
	}

	post, err := s.ownPost(ctx, actor, postID)
	if err != nil {
		return err
	}

	return s.store.DeleteStyleReviews(ctx, post.ID)
}
