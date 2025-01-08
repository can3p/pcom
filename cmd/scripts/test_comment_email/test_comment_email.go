package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// MockSender implements sender.Sender interface to capture email content
type MockSender struct{}

func (s *MockSender) Send(ctx context.Context, exec boil.ContextExecutor, id string, template string, mail *sender.Mail) error {
	fmt.Printf("=== Email Details ===\n")
	fmt.Printf("From: %s <%s>\n", mail.From.Name, mail.From.Address)
	fmt.Printf("To: %v\n", mail.To)
	fmt.Printf("Subject: %s\n", mail.Subject)
	fmt.Printf("\n=== Plain Text Content ===\n")
	fmt.Printf("%s\n", mail.Text)
	fmt.Printf("\n=== HTML Content ===\n")
	fmt.Printf("%s\n", mail.Html)
	return nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run cmd/scripts/test_comment_email.go <comment_id>")
	}
	commentID := os.Args[1]

	// Connect to database
	db := sqlx.MustConnect("postgres", os.Getenv("DATABASE_URL")+"?sslmode=disable")
	defer db.Close() //nolint:errcheck

	ctx := context.Background()

	// Load comment with all necessary relations
	comment, err := core.PostComments(
		core.PostCommentWhere.ID.EQ(commentID),
		qm.Load(qm.Rels(
			core.PostCommentRels.Post,
			core.PostRels.URL)),
		qm.Load(qm.Rels(
			core.PostCommentRels.Post,
			core.PostRels.User)),
		qm.Load(core.PostCommentRels.User),
	).One(ctx, db)
	if err != nil {
		log.Fatal(err)
	}

	commentAuthor := comment.R.User
	post := comment.R.Post

	// we can use author of the post as the participant
	participant := post.R.User
	participant.ID = "random-user-id"

	// Set up mock sender and media replacer
	mockSender := &MockSender{}

	// Set sender address for testing
	os.Setenv("SENDER_ADDRESS", "noreply@example.com") //nolint:errcheck

	fmt.Printf("\n=== Generating email for participant: %s ===\n", participant.Username)
	err = mail.PostCommentParticipants(ctx, db, mockSender, links.MediaReplacer, commentAuthor, participant, post, comment)
	if err != nil {
		log.Fatal(err)
	}
}
