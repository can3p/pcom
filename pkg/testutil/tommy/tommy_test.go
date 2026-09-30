package tommy_test

import (
	"context"
	"net/http"
	"net/mail"
	"os"
	"path"
	"strings"
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

	got := tm.Mails(t, "tommy-smoke@pcom.test", nil)
	require.Len(t, got, 1)
	require.Equal(t, "pcom@pcom.test", got[0].From)
	require.Equal(t, "smoke", got[0].Subject)
	require.Equal(t, "hello", got[0].Text)

	// the API's to filter is a substring match; the helpers keep the exact recipient
	tm.NoMails(t, "smoke@pcom.test", nil)
	require.Empty(t, tm.SettledMails(t, "tommy-smoke@pcom.test", func(m tommy.Mail) bool { return m.Subject != "smoke" }))
}

// TestS3Objects lists objects by prefix and reads one back, with the content
// type the client stored.
func TestS3Objects(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}

	tm := tommy.Shared(t)

	for _, key := range []string{"s3objects/a.png", "s3objects/b.webp", "other/c.png"} {
		req, err := http.NewRequest(http.MethodPut, tm.S3URL+"/"+tommy.Bucket+"/"+key, strings.NewReader("abc"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "image/"+strings.TrimPrefix(path.Ext(key), "."))

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	require.ElementsMatch(t, []tommy.ObjectInfo{
		{Key: "s3objects/a.png", Size: 3, ContentType: "image/png"},
		{Key: "s3objects/b.webp", Size: 3, ContentType: "image/webp"},
	}, tm.S3Objects(t, "s3objects/"))
	require.Equal(t, tommy.ObjectInfo{Key: "other/c.png", Size: 3, ContentType: "image/png"}, tm.S3Object(t, "other/c.png"))
}
