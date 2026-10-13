package app

import (
	"testing"

	"github.com/can3p/pcom/pkg/proofread/claude"
	"github.com/stretchr/testify/require"
)

func TestProofreadModel_OffWithoutKey(t *testing.T) {
	require.Nil(t, proofreadModel(claude.Options{}))
	require.NotNil(t, proofreadModel(claude.Options{APIKey: "sk-key"}))
}
