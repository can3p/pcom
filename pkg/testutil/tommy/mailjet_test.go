package tommy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"net/url"
	"testing"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/gogo/sender/mailjet"
	mjconfig "github.com/can3p/gogo/sender/mailjet/config"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/stretchr/testify/require"
)

// TestMailjetCredentialsReachTheAPI checks that MJ_APIKEY_PUBLIC and
// MJ_APIKEY_PRIVATE are the basic-auth user and password of the send request,
// in that order: tommy records what the request presented.
func TestMailjetCredentialsReachTheAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}

	tm := tommy.Shared(t)
	to := "mj-credentials@pcom.test"

	s := mailjet.NewSenderFromConfig(&mjconfig.Config{ApiKeyPublic: "public-key-1", ApiKeyPrivate: "private-key-2", BaseURL: tm.MailjetURL})
	require.NoError(t, s.Send(context.Background(), &sender.Mail{
		From:    mail.Address{Address: "pcom@pcom.test"},
		To:      []mail.Address{{Address: to}},
		Subject: "credentials",
		Text:    "hello",
	}))

	resp, err := http.Get(tm.APIURL + "/mail/messages?to=" + url.QueryEscape(to))
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	var got []struct {
		Meta struct {
			Public  string `json:"presented_api_key"`
			Private string `json:"presented_secret_key"`
		} `json:"meta"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	require.Equal(t, "public-key-1", got[0].Meta.Public)
	require.Equal(t, "private-key-2", got[0].Meta.Private)
}
