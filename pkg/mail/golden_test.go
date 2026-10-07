package mail_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

// mailsToGolden serializes fakesender recordings to a human-readable format for golden testing
func mailsToGolden(sent []fakesender.Recorded) []byte {
	var buf bytes.Buffer
	for i, rec := range sent {
		if i > 0 {
			buf.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&buf, "EmailType: %s\n", rec.EmailType)
		fmt.Fprintf(&buf, "UniqueID: %s\n", rec.UniqueID)
		fmt.Fprintf(&buf, "From: %s <%s>\n", rec.Mail.From.Name, rec.Mail.From.Address)
		if len(rec.Mail.To) > 0 {
			fmt.Fprintf(&buf, "To: %s\n", rec.Mail.To[0].Address)
		}
		fmt.Fprintf(&buf, "Subject: %s\n", rec.Mail.Subject)
		fmt.Fprintf(&buf, "\n--- TEXT ---\n%s\n", rec.Mail.Text)
		fmt.Fprintf(&buf, "\n--- HTML ---\n%s\n", rec.Mail.Html)
	}
	return buf.Bytes()
}

func TestMails_Goldens(t *testing.T) {
	t.Parallel()

	for _, m := range mail.All() {
		for _, sample := range m.Samples {
			t.Run(sample.Name, func(t *testing.T) {
				t.Parallel()

				require.NoError(t, sample.Err)

				s := fakesender.New()
				require.NoError(t, send(context.Background(), s, sample.Envelope))
				golden.Assert(t, sample.Name, mailsToGolden(s.Sent()))
			})
		}
	}
}

func TestMails_Registry(t *testing.T) {
	t.Parallel()

	all := mail.All()
	require.NotEmpty(t, all)

	seen := map[string]bool{}
	for _, m := range all {
		require.NotEmpty(t, m.Samples, "mail %s has no samples", m.Name)

		for _, sample := range m.Samples {
			require.False(t, seen[sample.Name], "sample %s is declared twice", sample.Name)
			seen[sample.Name] = true
		}
	}
}
