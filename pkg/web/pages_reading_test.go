package web

import (
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/stretchr/testify/require"
)

// A share link's page shows the shared post and its author to whoever holds
// the link, titled by the post's subject.
func TestSharedPost(t *testing.T) {
	t.Parallel()

	author := &model.User{ID: "author", Username: "alice"}

	cases := []struct {
		name    string
		subject *string
		want    string
	}{
		{"subject", new("Hello"), "Hello"},
		{"no subject", nil, "No Subject"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			post := &model.Post{ID: "post", Subject: tc.subject}
			page := SharedPost(newTestContext(t, http.MethodGet, "/shared/x"), nil, &shares.Shared{Post: post, Author: author})

			require.Same(t, post, page.Post)
			require.Same(t, author, page.Author)
			require.Equal(t, tc.want, page.PostSubject)
			require.Equal(t, tc.want, page.Name)
		})
	}
}
