package factory

import (
	"context"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
)

// Subscribable makes the user's journal public with email subscriptions
// turned on, so readers may subscribe to it.
func Subscribable() UserOpt {
	return func(u *model.User) {
		u.ProfileVisibility = model.ProfileVisibilityPublic
		u.EmailSubscriptionsEnabled = true
	}
}

// SubscribersMailedAt marks the post as already mailed to its author's email
// subscribers at t.
func SubscribersMailedAt(t time.Time) PostOpt {
	return func(p *model.Post) {
		p.SubscribersMailedAt = new(t)
	}
}

// EmailSubscriptionOpt customizes an EmailSubscription before it is inserted.
type EmailSubscriptionOpt func(*model.EmailSubscription)

// PendingSince leaves the subscription pending, its confirmation mailed at t.
func PendingSince(t time.Time) EmailSubscriptionOpt {
	return func(s *model.EmailSubscription) {
		s.Status = model.SubscriptionStatusPending
		s.ConfirmSentAt = t
		s.ConfirmedAt = nil
	}
}

// ConfirmedAt sets when the subscription was confirmed, for data that
// depends on the order of subscribers.
func ConfirmedAt(t time.Time) EmailSubscriptionOpt {
	return func(s *model.EmailSubscription) {
		s.ConfirmedAt = new(t)
	}
}

// EmailSubscription subscribes email to authorID's posts, active unless an
// option says otherwise. The subscriber row is created on first use of the
// address and shared by its later subscriptions; it is returned in
// Subscriber.
func EmailSubscription(ctx context.Context, exec repo.Executor, authorID, email string, opts ...EmailSubscriptionOpt) (*model.EmailSubscription, error) {
	store := repo.Using(exec)
	addr := emailaddr.Parse(email)

	subscriber, err := store.SubscriberByEmail(ctx, addr.Stored)
	if errors.Is(err, repo.ErrNotFound) {
		subscriber = &model.EmailSubscriber{Email: addr.Stored, EmailCanonical: addr.Canonical}
		if subscriber.ID, err = newID(); err != nil {
			return nil, err
		}

		if subscriber.ManageToken, err = newID(); err != nil {
			return nil, err
		}

		err = insert(ctx, exec, subscriber)
	}

	if err != nil {
		return nil, err
	}

	now := time.Now()
	sub := &model.EmailSubscription{
		SubscriberID: subscriber.ID, AuthorID: authorID, Status: model.SubscriptionStatusActive,
		ConfirmSentAt: now, ConfirmedAt: new(now),
	}

	if sub.ID, err = newID(); err != nil {
		return nil, err
	}

	if sub.ConfirmToken, err = newID(); err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(sub)
	}

	if err := insert(ctx, exec, sub); err != nil {
		return nil, err
	}

	sub.Subscriber = subscriber

	return sub, nil
}

// GetEmailSubscription looks up an email subscription by id.
func GetEmailSubscription(ctx context.Context, exec repo.Executor, id string) (*model.EmailSubscription, error) {
	return noRows(repo.Using(exec).EmailSubscriptionByID(ctx, id))
}

// GetEmailSubscriber looks up an email subscriber by their stored address.
func GetEmailSubscriber(ctx context.Context, exec repo.Executor, email string) (*model.EmailSubscriber, error) {
	return noRows(repo.Using(exec).SubscriberByEmail(ctx, email))
}

// ListEmailSubscriptions returns the subscriber's subscriptions, pending and
// active.
func ListEmailSubscriptions(ctx context.Context, exec repo.Executor, subscriberID string) ([]*model.EmailSubscription, error) {
	var subs []*model.EmailSubscription
	err := repo.Query(exec).NewSelect().Model(&subs).Where("subscriber_id = ?", subscriberID).Scan(ctx)

	return subs, err
}
