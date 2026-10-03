package posts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

// Action is what the author asked to do when saving.
type Action string

const (
	ActionSavePost  Action = "save_post"
	ActionMakeDraft Action = "make_draft"
	ActionPublish   Action = "publish"
	ActionDelete    Action = "delete"
	ActionAutosave  Action = "autosave"
)

// SaveInput is a post as the author submitted it.
type SaveInput struct {
	// PostID is the post to change, empty for a new one.
	PostID string
	// PromptID is the prompt a new post answers, if any.
	PromptID   string
	Subject    string
	URL        string
	Body       string
	Visibility core.PostVisibility
	Action     Action
}

// Saved is what a save left behind. Post is nil after a delete.
type Saved struct {
	Post    *core.Post
	Created bool
	Deleted bool
}

// ValidateSave checks the fields of a save and returns the message of every
// field that is wrong, by field name. Field names match the form's inputs.
func (s *Service) ValidateSave(in SaveInput) map[string]string {
	errs := map[string]string{}

	if err := validation.ValidateMinMax("subject", in.Subject, 0, s.limits.PostSubjectMaxLength); err != nil {
		errs["subject"] = err.Error()
	}

	if err := validation.ValidateURL(in.URL); err != nil {
		errs["url"] = err.Error()
	}

	if err := validation.ValidateMinMax("body", in.Body, 0, s.limits.PostBodyMaxLength); err != nil {
		errs["body"] = err.Error()
	}

	action := in.Action

	if action == "" {
		action = ActionAutosave
	}

	if err := validation.ValidateEnum(action,
		[]Action{ActionSavePost, ActionMakeDraft, ActionPublish, ActionDelete, ActionAutosave},
		[]string{"Save Post", "Make draft", "Publish"}); err != nil {
		errs["save_action"] = err.Error()
	}

	if err := validation.ValidateEnum(in.Visibility,
		[]core.PostVisibility{core.PostVisibilityDirectOnly, core.PostVisibilitySecondDegree, core.PostVisibilityPublic},
		[]string{"direct only", "their connections as well", "public"}); err != nil {
		errs["visibility"] = err.Error()
	}

	return errs
}

// Save creates or changes the actor's post, or deletes it. Publishing
// notifies the author's direct connections, and the asker of the prompt the
// post answers, in the same transaction.
func (s *Service) Save(ctx context.Context, actor *core.User, in SaveInput) (*Saved, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	var saved *Saved

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		var err error
		saved, err = s.save(ctx, tx, actor, in)

		return err
	})
	if err != nil {
		return nil, err
	}

	return saved, nil
}

func (s *Service) save(ctx context.Context, tx *repo.Store, actor *core.User, in SaveInput) (*Saved, error) {
	action := in.Action

	if action == "" {
		action = ActionAutosave
	}

	var existing *core.Post

	if in.PostID != "" {
		var err error

		existing, err = tx.OwnPost(ctx, in.PostID, actor.ID)
		if errors.Is(err, repo.ErrNotFound) {
			return nil, service.ErrNotFound
		} else if err != nil {
			return nil, err
		}
	}

	// the post is looked up first, so a stranger learns nothing from the
	// validation of what they sent
	errs := s.ValidateSave(in)

	for _, field := range []string{"subject", "url", "body", "save_action", "visibility"} {
		if msg, ok := errs[field]; ok {
			return nil, service.Invalid(field, msg)
		}
	}

	if in.Action == ActionDelete {
		// deleting a post that was never saved stores nothing
		if existing != nil {
			if err := tx.DeletePost(ctx, existing.ID); err != nil {
				return nil, err
			}
		}

		return &Saved{Deleted: true}, nil
	}

	prompt, err := s.promptOf(ctx, tx, actor, existing, in.PromptID)
	if err != nil {
		return nil, err
	}

	post := &core.Post{
		Subject:          null.NewString(strings.TrimSpace(in.Subject), strings.TrimSpace(in.Subject) != ""),
		Body:             strings.TrimSpace(in.Body),
		UserID:           actor.ID,
		VisibilityRadius: in.Visibility,
	}

	if url := strings.TrimSpace(in.URL); url != "" {
		storedURL, err := tx.StoreURL(ctx, url)
		if err != nil {
			return nil, err
		}

		post.URLID = null.StringFrom(storedURL.ID)
		// the relation is at hand, so the mails don't need to load it
		post.R = post.R.NewStruct()
		post.R.URL = storedURL
	}

	publishing := false

	if existing == nil {
		postID, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}

		post.ID = postID.String()

		if action == ActionPublish {
			// not null value means a published post
			post.PublishedAt = null.TimeFrom(time.Now())
			publishing = true
		}

		if err := tx.InsertPost(ctx, post); err != nil {
			return nil, err
		}

		if prompt != nil {
			prompt.Prompt.PostID = null.StringFrom(post.ID)

			if err := tx.UpdatePrompt(ctx, prompt.Prompt); err != nil {
				return nil, err
			}
		}
	} else {
		post.ID = existing.ID

		switch action {
		case ActionMakeDraft:
			post.PublishedAt = null.Time{}
		case ActionPublish:
			// publishing a published post is a plain save: it keeps its
			// date and doesn't notify anybody again
			if existing.PublishedAt.Valid {
				post.PublishedAt = existing.PublishedAt
			} else {
				post.PublishedAt = null.TimeFrom(time.Now())
				publishing = true
			}
		default:
			post.PublishedAt = existing.PublishedAt
		}

		if err := tx.UpdatePost(ctx, post); err != nil {
			return nil, err
		}
	}

	if publishing {
		if err := s.notifyPublished(ctx, tx, actor, post, prompt); err != nil {
			return nil, err
		}
	}

	return &Saved{Post: post, Created: existing == nil}, nil
}

// promptOf finds the prompt a post answers: the one being answered by a new
// post, or the one an existing post is linked to.
func (s *Service) promptOf(ctx context.Context, tx *repo.Store, actor *core.User, existing *core.Post, promptID string) (*postops.PostPrompt, error) {
	if existing != nil {
		return promptByPost(ctx, tx, existing.ID)
	}

	if promptID == "" {
		return nil, nil
	}

	return promptForRecipient(ctx, tx, actor.ID, promptID)
}

// notifyPublished tells the asker of the prompt the post answers, then every
// direct connection of the author.
func (s *Service) notifyPublished(ctx context.Context, tx *repo.Store, actor *core.User, post *core.Post, prompt *postops.PostPrompt) error {
	if prompt != nil && prompt.Prompt.DismissedAt.IsZero() {
		dbPrompt := prompt.Prompt

		dbPrompt.PostID = null.StringFrom(post.ID)
		dbPrompt.DismissedAt = null.TimeFrom(time.Now())

		if err := tx.UpdatePrompt(ctx, dbPrompt); err != nil {
			return err
		}

		if err := s.queue(ctx, tx, mail.PostPromptAnswer(s.ident.Site, s.ident.From, prompt.Author, actor, post, dbPrompt)); err != nil {
			return err
		}
	}

	directIDs, err := graph.DirectUserIDs(ctx, tx, post.UserID)
	if err != nil {
		return err
	}

	connections, err := tx.UsersByIDs(ctx, directIDs)
	if err != nil {
		return err
	}

	for _, conn := range connections {
		if err := s.queue(ctx, tx, mail.NewPost(s.ident.Site, s.ident.From, s.ident.Site.MediaReplacer, actor, conn, post)); err != nil {
			return err
		}
	}

	return nil
}

// EditView is a post as the edit page shows it.
type EditView struct {
	// Post has its author, stats and linked URL loaded.
	Post   *core.Post
	Prompt *postops.PostPrompt
}

// ForEdit loads the actor's post for the edit page: ErrNotFound if there is
// no such post, ErrForbidden if it is somebody else's.
func (s *Service) ForEdit(ctx context.Context, actor *core.User, postID string) (*EditView, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	post, err := s.store.PostForEdit(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if err := s.CheckEdit(ctx, actor, post); err != nil {
		return nil, err
	}

	prompt, err := promptByPost(ctx, s.store, post.ID)
	if err != nil {
		return nil, err
	}

	return &EditView{Post: post, Prompt: prompt}, nil
}

// CheckEdit reports whether the actor may edit the post: only its author may,
// as postops.GetPostCapabilities says. Anybody else gets ErrForbidden.
func (s *Service) CheckEdit(ctx context.Context, actor *core.User, post *core.Post) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	radius, err := graph.RadiusBetween(ctx, s.store, actor.ID, post.UserID)
	if err != nil {
		return err
	}

	if !postops.GetPostCapabilities(radius).CanEdit {
		return service.ErrForbidden
	}

	return nil
}

// Delete removes the actor's post, draft or published. Somebody else's post
// is not found.
func (s *Service) Delete(ctx context.Context, actor *core.User, postID string) error {
	return s.deleteOwn(ctx, actor, postID, false)
}

// DeleteDraft removes the actor's post if it is still a draft. What the
// author sees when there is no such draft is the database's own words, as it
// always was.
func (s *Service) DeleteDraft(ctx context.Context, actor *core.User, postID string) error {
	err := s.deleteOwn(ctx, actor, postID, true)
	if errors.Is(err, service.ErrNotFound) {
		return service.Invalid("postId", "sql: no rows in result set")
	}

	return err
}

func (s *Service) deleteOwn(ctx context.Context, actor *core.User, postID string, draftOnly bool) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		// only the author is allowed to delete a post, anybody else
		// should not even learn that the post exists
		post, err := tx.OwnPostForUpdate(ctx, postID, actor.ID, draftOnly)
		if errors.Is(err, repo.ErrNotFound) {
			return service.ErrNotFound
		} else if err != nil {
			return err
		}

		return tx.DeletePost(ctx, post.ID)
	})
}
