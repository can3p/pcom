package mail_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/stretchr/testify/require"
)

// The site a mail is formatted for decides its links: absolute ones start with
// the site root, uploaded media comes from the media CDN when there is one.
func TestSite_LinksAndMediaInMail(t *testing.T) {
	t.Parallel()

	const media = "3fa85f64-5717-4562-b3fc-2c963f66afa6.png"

	alice := &model.User{ID: "user-1", Email: "alice@example.test", Username: "alice"}
	bob := &model.User{ID: "user-2", Email: "bob@example.test", Username: "bob"}
	post := &model.Post{ID: "post-1", Body: "look ![pic](" + media + ")", UserID: alice.ID}

	for _, tc := range []struct {
		name      string
		site      links.Site
		wantLink  string
		wantMedia string
	}{
		{"root and CDN", links.Site{Root: "https://pcom.test", MediaCDN: "https://media.test"},
			"https://pcom.test" + links.Link("post", post.ID), "https://media.test/" + media},
		{"root without CDN", links.Site{Root: "https://pcom.test"},
			"https://pcom.test" + links.Link("post", post.ID), "https://pcom.test/user-media/" + media},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			require.NoError(t, deliver(context.Background(), s)(mail.NewPost(tc.site, testFrom, tc.site.MediaReplacer, alice, bob, post)))
			require.Len(t, s.Sent(), 1)

			m := s.Sent()[0].Mail
			require.Contains(t, m.Text, tc.wantLink)
			require.Contains(t, m.Html, tc.wantLink)
			require.Contains(t, m.Html, tc.wantMedia)
		})
	}
}
