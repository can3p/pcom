package web_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBindCapture_TextKeepsLF(t *testing.T) {
	t.Parallel()

	const sent, want = "One\r\nTwo\r\n\r\nThree\n", "One\nTwo\n\nThree\n"

	var multi bytes.Buffer

	mw := multipart.NewWriter(&multi)
	require.NoError(t, mw.WriteField(web.CaptureFieldText, sent))
	require.NoError(t, mw.Close())

	for _, r := range []struct {
		name, contentType, body string
	}{
		{"multipart", mw.FormDataContentType(), multi.String()},
		{"url-encoded", "application/x-www-form-urlencoded", url.Values{web.CaptureFieldText: {sent}}.Encode()},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(r.body))
			c.Request.Header.Set("Content-Type", r.contentType)

			in, err := web.BindCapture(c)
			require.NoError(t, err)
			require.Equal(t, want, in.Text)
		})
	}
}
