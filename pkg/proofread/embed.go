package proofread

import (
	_ "embed"
	"encoding/json"
)

var (
	//go:embed prompts/grammar.md
	grammarPrompt string
	//go:embed prompts/style.md
	stylePrompt string
	//go:embed schemas/grammar.json
	grammarSchema []byte
	//go:embed schemas/style.json
	styleSchema []byte
)

// GrammarSystemPrompt is the system prompt of a grammar check.
func GrammarSystemPrompt() string { return grammarPrompt }

// StyleSystemPrompt is the system prompt of a style review.
func StyleSystemPrompt() string { return stylePrompt }

// GrammarSchema is the JSON schema of GrammarOutput, for the model's
// structured output. Each call returns a fresh copy.
func GrammarSchema() map[string]any { return mustSchema(grammarSchema) }

// StyleSchema is the JSON schema of StyleOutput, for the model's structured
// output. Each call returns a fresh copy.
func StyleSchema() map[string]any { return mustSchema(styleSchema) }

func mustSchema(raw []byte) map[string]any {
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		panic("proofread: embedded schema is not JSON: " + err.Error())
	}

	return s
}
