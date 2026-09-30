package repo

import (
	"context"

	"github.com/can3p/gogo/sender"
)

// SendMail hands a mail to the sender on the store's executor. With the
// queueing sender that is an insert, so a service that sends inside Tx sends
// only if the transaction commits.
func (s *Store) SendMail(ctx context.Context, snd sender.Sender, uniqueID, emailType string, mail *sender.Mail) error {
	return snd.Send(ctx, s.exec, uniqueID, emailType, mail)
}
