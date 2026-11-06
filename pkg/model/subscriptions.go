package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// SubscriptionStatus is where an email subscription stands: pending until the
// subscriber confirms it, then active.
type SubscriptionStatus string

// The values of SubscriptionStatus.
const (
	SubscriptionStatusPending SubscriptionStatus = "pending"
	SubscriptionStatusActive  SubscriptionStatus = "active"
)

// SubscriptionMailKind is the kind of event a subscription_mail_log row
// counts: a mail sent, or a subscribe attempt refused for its domain.
type SubscriptionMailKind string

// The values of SubscriptionMailKind. SubscriptionMailKindDomainRefused is a
// subscribe attempt whose address's domain can't receive mail: no mail is
// sent, but the DNS lookup counts toward the site-wide hourly brake.
const (
	SubscriptionMailKindConfirm       SubscriptionMailKind = "confirm"
	SubscriptionMailKindManageLink    SubscriptionMailKind = "manage_link"
	SubscriptionMailKindDomainRefused SubscriptionMailKind = "domain_refused"
)

// EmailSubscriber is a row of email_subscribers: a reader without an account
// who gets authors' posts by email. ManageToken opens their subscriptions page.
type EmailSubscriber struct {
	bun.BaseModel `bun:"table:email_subscribers"`

	ID             string    `bun:"id,pk"`
	Email          string    `bun:"email"`
	EmailCanonical string    `bun:"email_canonical"`
	ManageToken    string    `bun:"manage_token"`
	CreatedAt      time.Time `bun:"created_at,default:now()"`

	Subscriptions []*EmailSubscription `bun:"rel:has-many,join:id=subscriber_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *EmailSubscriber) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}

// EmailSubscription is a row of email_subscriptions: one subscriber's
// subscription to one author.
type EmailSubscription struct {
	bun.BaseModel `bun:"table:email_subscriptions"`

	ID            string             `bun:"id,pk"`
	SubscriberID  string             `bun:"subscriber_id"`
	AuthorID      string             `bun:"author_id"`
	Status        SubscriptionStatus `bun:"status"`
	ConfirmToken  string             `bun:"confirm_token"`
	ConfirmSentAt time.Time          `bun:"confirm_sent_at"`
	ConfirmedAt   *time.Time         `bun:"confirmed_at"`
	CreatedAt     time.Time          `bun:"created_at,default:now()"`

	Subscriber *EmailSubscriber `bun:"rel:belongs-to,join:subscriber_id=id"`
	Author     *User            `bun:"rel:belongs-to,join:author_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *EmailSubscription) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}

// SubscriptionMailLog is a row of subscription_mail_log: one confirmation or
// manage-link mail sent to a mailbox, or one subscribe attempt refused for its
// domain, counted by the sending limits.
type SubscriptionMailLog struct {
	bun.BaseModel `bun:"table:subscription_mail_log"`

	EmailCanonical string               `bun:"email_canonical"`
	Kind           SubscriptionMailKind `bun:"kind"`
	SentAt         time.Time            `bun:"sent_at"`
}

// PowChallengeUsed is a row of pow_challenges_used: the salt of a
// proof-of-work challenge a form accepted, so it is used once.
type PowChallengeUsed struct {
	bun.BaseModel `bun:"table:pow_challenges_used"`

	Salt   string    `bun:"salt,pk"`
	UsedAt time.Time `bun:"used_at"`
}
