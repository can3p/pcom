package postops_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/postops"
	"github.com/stretchr/testify/require"
)

func TestDeserializePost_ErrorCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "invalid original_id",
			content: `---
original_id: not-a-uuid
---

body`,
			wantErr: "Invalid ID",
		},
		{
			name: "invalid visibility",
			content: `---
visibility: not_a_real_visibility
---

body`,
			wantErr: "Invalid visibility value",
		},
		{
			name: "invalid publish date",
			content: `---
published: not-a-date
---

body`,
			wantErr: "Invalid publish date",
		},
		{
			name: "unknown header",
			content: `---
some_unknown_header: value
---

body`,
			wantErr: "Unknown header",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := postops.DeserializePost([]byte(tc.content))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
