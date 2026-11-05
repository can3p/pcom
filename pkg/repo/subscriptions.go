package repo

// The queries of email subscriptions (pkg/service/subscriptions):
// email_subscribers, email_subscriptions and subscription_mail_log.

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/uptrace/bun"
)

// LockSubscribeMailbox guards a mailbox's subscriptions and the counts behind
// its mail limits.
const LockSubscribeMailbox LockScope = "subscribe_mailbox"

// SubscriberByEmail returns the subscriber whose stored address is email, or
// ErrNotFound.
func (s *Store) SubscriberByEmail(ctx context.Context, email string) (*model.EmailSubscriber, error) {
	sub := new(model.EmailSubscriber)
	if err := s.query().NewSelect().Model(sub).Where("email = ?", email).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return sub, nil
}

// SubscriberByManageToken returns the subscriber the manage token belongs
// to, or ErrNotFound.
func (s *Store) SubscriberByManageToken(ctx context.Context, token string) (*model.EmailSubscriber, error) {
	sub := new(model.EmailSubscriber)
	if err := s.query().NewSelect().Model(sub).Where("manage_token = ?", token).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return sub, nil
}

// LockSubscriber reads the subscriber and locks its row until the
// transaction ends, or returns ErrNotFound. Every change to a subscription
// takes this lock first.
func (s *Store) LockSubscriber(ctx context.Context, id string) (*model.EmailSubscriber, error) {
	sub := new(model.EmailSubscriber)
	if err := s.query().NewSelect().Model(sub).Where("id = ?", id).For("UPDATE").Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return sub, nil
}

// InsertSubscriber inserts a new subscriber.
func (s *Store) InsertSubscriber(ctx context.Context, sub *model.EmailSubscriber) error {
	_, err := s.query().NewInsert().Model(sub).Exec(ctx)

	return err
}

// DeleteSubscriberIfEmpty deletes the subscriber unless a subscription of
// theirs is left.
func (s *Store) DeleteSubscriberIfEmpty(ctx context.Context, id string) error {
	_, err := s.query().NewDelete().Model((*model.EmailSubscriber)(nil)).
		Where("id = ?", id).
		Where("not exists (select 1 from email_subscriptions es where es.subscriber_id = ?TableAlias.id)").
		Exec(ctx)

	return err
}

// DeleteSubscribersWithoutSubscriptions deletes every subscriber who has no
// subscription left.
func (s *Store) DeleteSubscribersWithoutSubscriptions(ctx context.Context) (int64, error) {
	res, err := s.query().NewDelete().Model((*model.EmailSubscriber)(nil)).
		Where("not exists (select 1 from email_subscriptions es where es.subscriber_id = ?TableAlias.id)").
		Exec(ctx)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// EmailSubscriptionByID returns the subscription, or ErrNotFound.
func (s *Store) EmailSubscriptionByID(ctx context.Context, id string) (*model.EmailSubscription, error) {
	sub := new(model.EmailSubscription)
	if err := s.query().NewSelect().Model(sub).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return sub, nil
}

// EmailSubscriptionByConfirmToken returns the subscription the confirmation
// token was mailed for, or ErrNotFound.
func (s *Store) EmailSubscriptionByConfirmToken(ctx context.Context, token string) (*model.EmailSubscription, error) {
	sub := new(model.EmailSubscription)
	if err := s.query().NewSelect().Model(sub).Where("confirm_token = ?", token).Scan(ctx); err != nil {
		return nil, notFound(err)
	}

	return sub, nil
}

// EmailSubscriptionOf returns the subscriber's subscription to the author, or nil
// when there is none.
func (s *Store) EmailSubscriptionOf(ctx context.Context, subscriberID, authorID string) (*model.EmailSubscription, error) {
	sub := new(model.EmailSubscription)

	return orNil(sub, s.query().NewSelect().Model(sub).
		Where("subscriber_id = ?", subscriberID).
		Where("author_id = ?", authorID).
		Scan(ctx))
}

// EmailSubscriptionsOfSubscriber returns the subscriber's subscriptions in the
// status given, each with its author, by the author's username.
func (s *Store) EmailSubscriptionsOfSubscriber(ctx context.Context, subscriberID string, status model.SubscriptionStatus) ([]*model.EmailSubscription, error) {
	var subs []*model.EmailSubscription
	err := s.query().NewSelect().Model(&subs).
		Relation("Author").
		Where("?TableAlias.subscriber_id = ?", subscriberID).
		Where("?TableAlias.status = ?", status).
		Order("author.username").
		Scan(ctx)

	return subs, err
}

// EmailSubscriptionsOfAuthor returns the author's subscriptions in the status
// given, each with its subscriber, the most recently confirmed first.
func (s *Store) EmailSubscriptionsOfAuthor(ctx context.Context, authorID string, status model.SubscriptionStatus) ([]*model.EmailSubscription, error) {
	var subs []*model.EmailSubscription
	err := s.query().NewSelect().Model(&subs).
		Relation("Subscriber").
		Where("?TableAlias.author_id = ?", authorID).
		Where("?TableAlias.status = ?", status).
		OrderExpr("?TableAlias.confirmed_at DESC NULLS LAST, ?TableAlias.id").
		Scan(ctx)

	return subs, err
}

// EmailSubscriptionCount is how many of the author's subscriptions are in the
// status given.
func (s *Store) EmailSubscriptionCount(ctx context.Context, authorID string, status model.SubscriptionStatus) (int, error) {
	n, err := s.query().NewSelect().Model((*model.EmailSubscription)(nil)).
		Where("author_id = ?", authorID).
		Where("status = ?", status).
		Count(ctx)

	return int(n), err
}

// InsertEmailSubscription inserts a new subscription.
func (s *Store) InsertEmailSubscription(ctx context.Context, sub *model.EmailSubscription) error {
	_, err := s.query().NewInsert().Model(sub).Exec(ctx)

	return err
}

// SaveEmailSubscription writes the named columns of sub.
func (s *Store) SaveEmailSubscription(ctx context.Context, sub *model.EmailSubscription, columns ...string) error {
	_, err := s.query().NewUpdate().Model(sub).WherePK().Column(columns...).Exec(ctx)

	return err
}

// DeleteEmailSubscription deletes the subscription.
func (s *Store) DeleteEmailSubscription(ctx context.Context, id string) error {
	_, err := s.query().NewDelete().Model((*model.EmailSubscription)(nil)).Where("id = ?", id).Exec(ctx)

	return err
}

// DeletePendingEmailSubscriptionsSentBefore deletes the pending subscriptions
// whose confirmation was mailed before t.
func (s *Store) DeletePendingEmailSubscriptionsSentBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := s.query().NewDelete().Model((*model.EmailSubscription)(nil)).
		Where("status = ?", model.SubscriptionStatusPending).
		Where("confirm_sent_at < ?", t).
		Exec(ctx)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// LogSubscriptionMail records a confirmation or manage-link mail.
func (s *Store) LogSubscriptionMail(ctx context.Context, row *model.SubscriptionMailLog) error {
	_, err := s.query().NewInsert().Model(row).Exec(ctx)

	return err
}

// SubscriptionMailsTo counts the mails of kind sent to the mailbox canonical
// since t.
func (s *Store) SubscriptionMailsTo(ctx context.Context, canonical string, kind model.SubscriptionMailKind, since time.Time) (int, error) {
	n, err := s.subscriptionMails(kind, since).Where("email_canonical = ?", canonical).Count(ctx)

	return int(n), err
}

// SubscriptionMailsSince counts the mails of kind sent to anybody since t.
func (s *Store) SubscriptionMailsSince(ctx context.Context, kind model.SubscriptionMailKind, since time.Time) (int, error) {
	n, err := s.subscriptionMails(kind, since).Count(ctx)

	return int(n), err
}

func (s *Store) subscriptionMails(kind model.SubscriptionMailKind, since time.Time) *bun.SelectQuery {
	return s.query().NewSelect().Model((*model.SubscriptionMailLog)(nil)).
		Where("kind = ?", kind).
		Where("sent_at >= ?", since)
}

// DeleteSubscriptionMailsBefore deletes the mail log rows sent before t.
func (s *Store) DeleteSubscriptionMailsBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := s.query().NewDelete().Model((*model.SubscriptionMailLog)(nil)).
		Where("sent_at < ?", t).
		Exec(ctx)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// MarkPostSubscribersMailed records that the post's subscribers were mailed
// at t, and reports false when they already were.
func (s *Store) MarkPostSubscribersMailed(ctx context.Context, postID string, t time.Time) (bool, error) {
	res, err := s.query().NewUpdate().Model((*model.Post)(nil)).
		Set("subscribers_mailed_at = ?", t).
		Where("id = ?", postID).
		Where("subscribers_mailed_at IS NULL").
		Exec(ctx)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
