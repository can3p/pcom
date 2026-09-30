package tommy

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	// MailDeadline is how long Mails waits for the expected mail to arrive.
	MailDeadline = 5 * time.Second

	pollEvery = 100 * time.Millisecond
)

// Mail is one message tommy received.
type Mail struct {
	From string
	// To holds every recipient: To, Cc and Bcc.
	To      []string
	Subject string
	Text    string
	HTML    string
}

// SentTo reports whether addr is one of the recipients, ignoring case.
func (m Mail) SentTo(addr string) bool {
	for _, to := range m.To {
		if strings.EqualFold(to, addr) {
			return true
		}
	}

	return false
}

// Mails waits up to MailDeadline for mail sent to the address to that match
// accepts (nil accepts any) and returns every such mail, newest first. It
// fails the test if none arrives.
func (tm *Tommy) Mails(t testing.TB, to string, match func(Mail) bool) []Mail {
	t.Helper()

	deadline := time.Now().Add(MailDeadline)

	for {
		if got := tm.ListMails(t, to, match); len(got) > 0 {
			return got
		}

		if time.Now().After(deadline) {
			t.Fatalf("tommy: no matching mail to %q within %v; mail to it: %v", to, MailDeadline, subjects(tm.ListMails(t, to, nil)))
		}

		time.Sleep(pollEvery)
	}
}

// NoMails fails the test if there is mail to the address to that match
// accepts (nil accepts any). tommy can't tell whether mail is still on its
// way, so the caller first makes sure the sender is done.
func (tm *Tommy) NoMails(t testing.TB, to string, match func(Mail) bool) {
	t.Helper()

	if got := tm.ListMails(t, to, match); len(got) > 0 {
		t.Fatalf("tommy: expected no matching mail to %q, got %v", to, subjects(got))
	}
}

// ListMails returns the mail received so far that was sent to the address
// to and that match accepts (nil accepts any), newest first. An empty to
// means any recipient; then match tells this test's mail apart from other
// tests'. tommy's to filter is a substring match, so the exact recipient is
// checked again here.
func (tm *Tommy) ListMails(t testing.TB, to string, match func(Mail) bool) []Mail {
	t.Helper()

	type address struct {
		Email string `json:"email"`
	}

	var got []struct {
		Message struct {
			From    address   `json:"from"`
			To      []address `json:"to"`
			Cc      []address `json:"cc"`
			Bcc     []address `json:"bcc"`
			Subject string    `json:"subject"`
			Text    string    `json:"text"`
			HTML    string    `json:"html"`
		} `json:"message"`
	}

	u := tm.APIURL + "/mail/messages"
	if to != "" {
		u += "?to=" + url.QueryEscape(to)
	}

	getJSON(t, u, &got)

	var mails []Mail

	for _, v := range got {
		msg := v.Message
		m := Mail{From: msg.From.Email, Subject: msg.Subject, Text: msg.Text, HTML: msg.HTML}

		for _, list := range [][]address{msg.To, msg.Cc, msg.Bcc} {
			for _, a := range list {
				m.To = append(m.To, a.Email)
			}
		}

		if to != "" && !m.SentTo(to) {
			continue
		}

		if match != nil && !match(m) {
			continue
		}

		mails = append(mails, m)
	}

	return mails
}

func subjects(mails []Mail) []string {
	out := make([]string, 0, len(mails))
	for _, m := range mails {
		out = append(out, m.Subject)
	}

	return out
}
