// Package seed fills a development database with a small, named world that
// exercises every feature of pcom. Run it with
//
//	go run ./cmd/web seed [--reset]
//
// (the seed command is a thin wrapper around Run.) It takes DATABASE_URL (and
// SITE_ROOT, default http://localhost:8080) from the settings. It refuses to
// run when Options.Production is set (FLY_APP_NAME is set). Without --reset it exits if users already exist; --reset truncates
// every table except migrations and system_settings first.
//
// Every account logs in with a code mailed to <username>@example.test:
//
//	alice  alice@example.test  connections       connected to bob; second degree to carol; owns the API key
//	bob    bob@example.test    registered_users  connected to alice and carol
//	carol  carol@example.test  public            connected to bob
//	dave   dave@example.test   connections       unrelated to everyone
//	eve    eve@example.test    registered_users  has an unaccepted invite (eve-friend@example.test)
//
// Alice's API key is the fixed value
//
//	00000000-0000-4000-8000-000000000001
//
// for blg development. Registration is opened in the seeded database only.
package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
)

const (
	// AliceAPIKey is Alice's fixed API key.
	AliceAPIKey = "00000000-0000-4000-8000-000000000001"
	// FeedURL is the placeholder feed; the poller fails on it harmlessly.
	FeedURL = "https://example.test/seed/feed.xml"
	// DefaultSiteRoot is used when SITE_ROOT is not set.
	DefaultSiteRoot = "http://localhost:8080"
)

// Usernames returns the seeded usernames; each user's email is
// <username>@example.test.
func Usernames() []string {
	return []string{"alice", "bob", "carol", "dave", "eve"}
}

// Options configures Run.
type Options struct {
	Reset bool
	// Production refuses to seed: the database looks like a live site.
	Production bool
	SiteRoot   string
}

type seededUser struct {
	Name       string
	Visibility core.ProfileVisibility
	Role       string
	user       *core.User
}

// Run seeds the database behind db and prints a summary to out. Everything
// happens in one transaction, so a failure changes nothing.
func Run(ctx context.Context, db *sql.DB, out io.Writer, opts Options) (err error) {
	if opts.Production {
		return errors.New("refusing to seed: FLY_APP_NAME is set, this looks like production")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if opts.Reset {
		if err = truncateAll(ctx, tx); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
	} else {
		var exist bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&exist); err != nil {
			return err
		}

		if exist {
			return errors.New("users already exist: run with --reset to wipe the database first")
		}
	}

	users, key, err := build(ctx, tx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	siteRoot := opts.SiteRoot
	if siteRoot == "" {
		siteRoot = DefaultSiteRoot
	}

	_, err = io.WriteString(out, summary(users, key, siteRoot))

	return err
}

func truncateAll(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT quote_ident(table_name) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'
		AND table_name NOT IN ('migrations', 'system_settings')`)
	if err != nil {
		return err
	}

	defer func() { _ = rows.Close() }()

	var tables []string

	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return err
		}

		tables = append(tables, t)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if len(tables) == 0 {
		return nil
	}

	_, err = tx.ExecContext(ctx, "TRUNCATE "+strings.Join(tables, ", ")+" CASCADE")

	return err
}

func build(ctx context.Context, tx *sql.Tx) ([]*seededUser, string, error) {
	users := []*seededUser{
		{Name: "alice", Visibility: core.ProfileVisibilityConnections, Role: "connected to bob; second degree to carol; owns the API key"},
		{Name: "bob", Visibility: core.ProfileVisibilityRegisteredUsers, Role: "connected to alice and carol"},
		{Name: "carol", Visibility: core.ProfileVisibilityPublic, Role: "connected to bob"},
		{Name: "dave", Visibility: core.ProfileVisibilityConnections, Role: "unrelated to everyone"},
		{Name: "eve", Visibility: core.ProfileVisibilityRegisteredUsers, Role: "has an unaccepted invite"},
	}

	byName := map[string]*core.User{}

	for _, su := range users {
		u, err := factory.User(ctx, tx,
			factory.WithUsername(su.Name),
			factory.WithEmail(su.Name+"@example.test"),
			factory.WithVisibility(su.Visibility),
		)
		if err != nil {
			return nil, "", fmt.Errorf("user %s: %w", su.Name, err)
		}

		su.user = u
		byName[su.Name] = u
	}

	alice, bob, carol, eve := byName["alice"], byName["bob"], byName["carol"], byName["eve"]

	if err := world(ctx, tx, alice, bob, carol, eve); err != nil {
		return nil, "", err
	}

	k, err := factory.APIKey(ctx, tx, alice.ID, factory.WithAPIKey(AliceAPIKey))
	if err != nil {
		return nil, "", err
	}

	return users, k.APIKey, nil
}

func world(ctx context.Context, tx *sql.Tx, alice, bob, carol, eve *core.User) error {
	if _, _, err := factory.Connect(ctx, tx, alice.ID, bob.ID); err != nil {
		return err
	}

	if _, _, err := factory.Connect(ctx, tx, bob.ID, carol.ID); err != nil {
		return err
	}

	if _, err := factory.Invitation(ctx, tx, eve.ID, factory.Sent("eve-friend@example.test")); err != nil {
		return err
	}

	// Posts: every visibility, a draft, a URL post, a share link.
	var pub *core.Post

	for _, v := range []core.PostVisibility{core.PostVisibilityDirectOnly, core.PostVisibilitySecondDegree, core.PostVisibilityPublic} {
		p, err := factory.Post(ctx, tx, alice.ID, factory.Published(), factory.Visibility(v),
			factory.WithSubject("Alice, "+v.String()), factory.WithBody("A "+v.String()+" post by Alice."))
		if err != nil {
			return err
		}

		if v == core.PostVisibilityPublic {
			pub = p
		}
	}

	if _, err := factory.Post(ctx, tx, alice.ID, factory.WithSubject("Alice, draft"), factory.WithBody("Not published yet.")); err != nil {
		return err
	}

	u, err := factory.NormalizedURL(ctx, tx)
	if err != nil {
		return err
	}

	if _, err := factory.Post(ctx, tx, bob.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly),
		factory.WithURL(u.ID), factory.WithSubject("Bob, link"), factory.WithBody("Worth a read.")); err != nil {
		return err
	}

	if _, err := factory.PostShare(ctx, tx, pub.ID); err != nil {
		return err
	}

	// A comment thread three levels deep on Alice's public post.
	c1, err := factory.Comment(ctx, tx, pub.ID, bob.ID, factory.WithCommentBody("Level 1 (bob)"))
	if err != nil {
		return err
	}

	c2, err := factory.Comment(ctx, tx, pub.ID, alice.ID, factory.ReplyTo(c1.ID), factory.WithCommentBody("Level 2 (alice)"))
	if err != nil {
		return err
	}

	if _, err := factory.Comment(ctx, tx, pub.ID, bob.ID, factory.ReplyTo(c2.ID), factory.WithCommentBody("Level 3 (bob)")); err != nil {
		return err
	}

	// An open prompt from Bob to Alice.
	if _, err := factory.PostPrompt(ctx, tx, bob.ID, alice.ID, factory.WithPromptMessage("What are you reading?")); err != nil {
		return err
	}

	// One feed with items inserted directly.
	feed, err := factory.RSSFeed(ctx, tx, factory.WithFeedURL(FeedURL), factory.WithFeedTitle("Seed feed"))
	if err != nil {
		return err
	}

	for _, title := range []string{"First seed item", "Second seed item", "Third seed item"} {
		if _, err := factory.RSSItem(ctx, tx, feed.ID, factory.WithItemTitle(title)); err != nil {
			return err
		}
	}

	if _, err := factory.Subscription(ctx, tx, alice.ID, feed.ID); err != nil {
		return err
	}

	// Alice asks Bob to introduce her to Carol; nobody has decided yet.
	if _, err := factory.MediationRequest(ctx, tx, alice.ID, carol.ID, factory.WithSourceNote("Hi Carol, Bob says we should meet.")); err != nil {
		return err
	}

	return factory.SetRegistrationOpen(ctx, tx, true)
}

func summary(users []*seededUser, apiKey, siteRoot string) string {
	var b strings.Builder

	fmt.Fprintln(&b, "Seeded the development database.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "%-6s %-24s %-17s %s\n", "user", "login (email)", "profile", "role")

	for _, su := range users {
		fmt.Fprintf(&b, "%-6s %-24s %-17s %s\n", su.Name, su.user.Email, su.Visibility, su.Role)
	}

	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "alice's API key: %s\n", apiKey)
	fmt.Fprintf(&b, "log in at: %s\n", strings.TrimRight(siteRoot, "/")+"/login")

	return b.String()
}
