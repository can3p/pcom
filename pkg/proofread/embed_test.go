package proofread_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/can3p/pcom/pkg/proofread"
	"github.com/stretchr/testify/require"
)

// jsonKeys is the JSON object keys v marshals to.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	return slices.Sorted(maps.Keys(m))
}

func properties(schema map[string]any, path ...string) map[string]any {
	for _, p := range path {
		schema = schema["properties"].(map[string]any)[p].(map[string]any)
		if items, ok := schema["items"]; ok {
			schema = items.(map[string]any)
		}
	}

	return schema["properties"].(map[string]any)
}

// The schemas the model answers by name every field the answer types read.
func TestSchemas_MatchTheAnswerTypes(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
		path   []string
		value  any
	}{
		{"grammar", proofread.GrammarSchema(), nil, proofread.GrammarOutput{}},
		{"grammar suggestion", proofread.GrammarSchema(), []string{"suggestions"}, proofread.Suggestion{}},
		{"style", proofread.StyleSchema(), nil, proofread.StyleOutput{}},
		{"style note", proofread.StyleSchema(), []string{"notes"}, proofread.Note{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, jsonKeys(t, tc.value), slices.Sorted(maps.Keys(properties(tc.schema, tc.path...))))
		})
	}
}
