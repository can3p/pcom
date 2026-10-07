package mail_test

import (
	"context"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
)

const testFrom = "noreply@pcom.test"

// setPostURL sets the URL relation on a post, as if it had been loaded.
func setPostURL(post *model.Post, url *model.NormalizedURL) {
	post.URL = url
}

// deliver queues out on s, the way the posts service does, so the tests read
// what a recipient would get. A nil out sends nothing.
func deliver(ctx context.Context, s *fakesender.Sender) func(*mail.Envelope) error {
	return func(out *mail.Envelope) error {
		if out == nil {
			return nil
		}

		return s.Send(ctx, nil, out.UniqueID, out.Type, out.Mail)
	}
}

// deliverE is deliver for the formatters that can fail.
func deliverE(ctx context.Context, s *fakesender.Sender) func(*mail.Envelope, error) error {
	return func(out *mail.Envelope, err error) error {
		if err != nil {
			return err
		}

		return deliver(ctx, s)(out)
	}
}

// send queues an envelope the way a service does, through the sender with no database.
func send(ctx context.Context, s *fakesender.Sender, e *mail.Envelope) error {
	return s.Send(ctx, nil, e.UniqueID, e.Type, e.Mail)
}
