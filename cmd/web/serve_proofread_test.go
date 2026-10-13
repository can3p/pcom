package main

import (
	"testing"
	"time"

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/proofread"
	"github.com/can3p/pcom/pkg/proofread/claude"
	proofreadsvc "github.com/can3p/pcom/pkg/service/proofread"
	"github.com/stretchr/testify/require"
)

// Every proofread setting reaches its own field of the app's config.
func TestProofreadConfig(t *testing.T) {
	got := proofreadConfig(config.Proofread{
		AnthropicKey:      settings.Secret("sk-key"),
		AnthropicEndpoint: "http://wiremock.test",
		GrammarModel:      "grammar-model",
		GrammarEffort:     "high",
		StyleModel:        "style-model",
		StyleEffort:       "low",
		GrammarPerHour:    1,
		StylePerHour:      2,
		SiteDailyRequests: 3,
		NoteMaxLength:     7,
		Timeout:           4 * time.Second,
		GrammarMaxTokens:  5,
		StyleMaxTokens:    6,
	})

	require.Equal(t, proofreadsvc.Limits{GrammarPerHour: 1, StylePerHour: 2, SiteDailyRequests: 3, NoteMaxLength: 7}, got.Limits)
	require.Equal(t, proofreadsvc.Models{
		Grammar: proofread.ModelChoice{Name: "grammar-model", Effort: proofread.EffortHigh},
		Style:   proofread.ModelChoice{Name: "style-model", Effort: proofread.EffortLow},
	}, got.Models)
	require.Equal(t, claude.Options{APIKey: "sk-key", Endpoint: "http://wiremock.test", Timeout: 4 * time.Second, GrammarMaxTokens: 5, StyleMaxTokens: 6}, got.Claude)
}
