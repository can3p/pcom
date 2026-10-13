// Package proofreadtest is an in-memory proofread.Model for tests.
package proofreadtest

import (
	"context"
	"sync"

	"github.com/can3p/pcom/pkg/proofread"
)

// DefaultInputTokens and DefaultOutputTokens are the usage the fake reports
// for every call.
const (
	DefaultInputTokens  = 100
	DefaultOutputTokens = 50
)

// Fake answers every call with the answer set for its kind (an empty one by
// default) and records the inputs. It is safe for concurrent use.
type Fake struct {
	mu          sync.Mutex
	grammar     proofread.GrammarOutput
	style       proofread.StyleOutput
	grammarErr  error
	styleErr    error
	grammarSeen []proofread.GrammarInput
	styleSeen   []proofread.StyleInput
}

var _ proofread.Model = (*Fake)(nil)

// New returns a fake that answers with no suggestions and no notes.
func New() *Fake { return &Fake{} }

// AnswerGrammar sets the answer of the next grammar checks.
func (f *Fake) AnswerGrammar(out proofread.GrammarOutput) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.grammar = out
}

// AnswerStyle sets the answer of the next style reviews.
func (f *Fake) AnswerStyle(out proofread.StyleOutput) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.style = out
}

// FailGrammarWith makes grammar checks fail with err; nil makes them answer
// again.
func (f *Fake) FailGrammarWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.grammarErr = err
}

// FailStyleWith makes style reviews fail with err; nil makes them answer
// again.
func (f *Fake) FailStyleWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.styleErr = err
}

// GrammarCalls returns the inputs of every grammar check so far.
func (f *Fake) GrammarCalls() []proofread.GrammarInput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]proofread.GrammarInput(nil), f.grammarSeen...)
}

// StyleCalls returns the inputs of every style review so far.
func (f *Fake) StyleCalls() []proofread.StyleInput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]proofread.StyleInput(nil), f.styleSeen...)
}

// Grammar records in and returns the grammar answer or error.
func (f *Fake) Grammar(_ context.Context, in proofread.GrammarInput) (proofread.GrammarOutput, proofread.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.grammarSeen = append(f.grammarSeen, in)
	if f.grammarErr != nil {
		return proofread.GrammarOutput{}, usage(in.Model, 0), f.grammarErr
	}

	return f.grammar, usage(in.Model, DefaultOutputTokens), nil
}

// Style records in and returns the style answer or error.
func (f *Fake) Style(_ context.Context, in proofread.StyleInput) (proofread.StyleOutput, proofread.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.styleSeen = append(f.styleSeen, in)
	if f.styleErr != nil {
		return proofread.StyleOutput{}, usage(in.Model, 0), f.styleErr
	}

	return f.style, usage(in.Model, DefaultOutputTokens), nil
}

func usage(m proofread.ModelChoice, output int) proofread.Usage {
	return proofread.Usage{Model: m.Name, InputTokens: DefaultInputTokens, OutputTokens: output}
}
