package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/repo"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run cmd/scripts/test_comment_email.go <comment_id>")
	}
	commentID := os.Args[1]

	// Connect to database
	store, closeDB, err := repo.ConnectPostgres(os.Getenv("DATABASE_URL") + "?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer closeDB() //nolint:errcheck

	ctx := context.Background()

	// Load comment with all necessary relations
	comment, err := store.CommentForMail(ctx, commentID)
	if err != nil {
		log.Fatal(err)
	}

	commentAuthor := comment.R.User
	post := comment.R.Post

	// we can use author of the post as the participant
	participant := post.R.User
	participant.ID = "random-user-id"

	// Set sender address for testing
	os.Setenv("SENDER_ADDRESS", "noreply@example.com") //nolint:errcheck

	fmt.Printf("\n=== Generating email for participant: %s ===\n", participant.Username)

	out, err := mail.PostCommentParticipants(links.MediaReplacer, commentAuthor, participant, post, comment)
	if err != nil {
		log.Fatal(err)
	}

	if out == nil {
		log.Fatal("nothing to send: the participant is the comment's author")
	}

	fmt.Printf("=== Email Details ===\n")
	fmt.Printf("From: %s <%s>\n", out.Mail.From.Name, out.Mail.From.Address)
	fmt.Printf("To: %v\n", out.Mail.To)
	fmt.Printf("Subject: %s\n", out.Mail.Subject)
	fmt.Printf("\n=== Plain Text Content ===\n")
	fmt.Printf("%s\n", out.Mail.Text)
	fmt.Printf("\n=== HTML Content ===\n")
	fmt.Printf("%s\n", out.Mail.Html)
}
