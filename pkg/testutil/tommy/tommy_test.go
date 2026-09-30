package tommy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"testing"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/gogo/sender/mailjet"
	mjconfig "github.com/can3p/gogo/sender/mailjet/config"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	_ = tommy.Cleanup()
	os.Exit(code)
}

// TestMailjetSenderDeliversToTommy is the path every mail of the app takes in
// development and tests: gogo's Mailjet sender, pointed at tommy by MJ_API_BASE.
func TestMailjetSenderDeliversToTommy(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}

	tm := tommy.Shared(t)

	s := mailjet.NewSenderFromConfig(&mjconfig.Config{ApiKeyPublic: "any", ApiKeyPrivate: "any", BaseURL: tm.MailjetURL})
	require.NoError(t, s.Send(context.Background(), &sender.Mail{
		From:    mail.Address{Address: "pcom@pcom.test"},
		To:      []mail.Address{{Address: "tommy-smoke@pcom.test"}},
		Subject: "smoke",
		Text:    "hello",
	}))

	resp, err := http.Get(tm.APIURL + "/mail/messages?to=" + url.QueryEscape("tommy-smoke@pcom.test"))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var got []struct {
		Message struct {
			Subject string `json:"subject"`
			Text    string `json:"text"`
		} `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	require.Equal(t, "smoke", got[0].Message.Subject)
	require.Equal(t, "hello", got[0].Message.Text)
}
