// Package fakesender is a fake of github.com/can3p/gogo/sender.Sender for
// unit and package tests: it records every mail instead of delivering it.
// From R2 on, real delivery in E2E and development paths is exercised
// through tommy instead; this fake stays the right tool for tests that only
// care what the code under test tried to send.
package fakesender

import (
	"context"
	"sync"

	"github.com/can3p/gogo/sender"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// compile-time check that Sender implements gogo/sender.Sender.
var _ sender.Sender = (*Sender)(nil)

// Recorded is one call to Send.
type Recorded struct {
	UniqueID  string
	EmailType string
	Mail      *sender.Mail
}

// Sender records every mail passed to Send. The zero value is ready to use;
// New is equivalent and reads slightly better at a call site. It is safe
// for concurrent use.
type Sender struct {
	mu   sync.Mutex
	sent []Recorded
	err  error
}

// New returns a Sender that records mail and never fails.
func New() *Sender {
	return &Sender{}
}

// FailWith makes every subsequent Send return err instead of recording the
// mail. Pass nil to go back to recording.
func (s *Sender) FailWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.err = err
}

// Send implements gogo/sender.Sender. exec is accepted, to match the
// interface, but is not used: this fake never touches the database.
func (s *Sender) Send(ctx context.Context, exec boil.ContextExecutor, uniqueID string, emailType string, mail *sender.Mail) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return s.err
	}

	s.sent = append(s.sent, Recorded{
		UniqueID:  uniqueID,
		EmailType: emailType,
		Mail:      mail,
	})

	return nil
}

// Sent returns every mail recorded so far, in the order Send was called.
func (s *Sender) Sent() []Recorded {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Recorded, len(s.sent))
	copy(out, s.sent)

	return out
}
