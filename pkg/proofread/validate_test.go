package proofread_test

import (
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/proofread"
	"github.com/stretchr/testify/require"
)

func TestLocate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		text, before, quote string
		ok                  bool
		start, end, occ     int
		needle              string
	}{
		{"english", "I has a cat", "I ", "has", true, 2, 5, 0, "I has"},
		{"cyrillic byte offsets", "Я иду домой и он идёт домой", "он идёт ", "домой", true, 38, 48, 0, "он идёт домой"},
		{"twice, before picks the second", "ab cd ab ef", "cd ", "ab", true, 6, 8, 0, "cd ab"},
		{"twice, no before takes the first", "ab cd ab ef", "", "ab", true, 0, 2, 0, "ab"},
		{"before does not help, quote alone", "ab cd ab ef", "zz ", "ef", true, 9, 11, 0, "ef"},
		{"made up", "ab cd", "", "xyz", false, 0, 0, 0, ""},
		{"empty quote", "ab cd", "ab", "", false, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, ok := proofread.Locate(tt.text, tt.before, tt.quote)
			require.Equal(t, tt.ok, ok)

			if ok {
				require.Equal(t, proofread.Match{Start: tt.start, End: tt.end, Needle: tt.needle, Occurrence: tt.occ}, m)
				require.Equal(t, tt.quote, tt.text[m.Start:m.End])
			}
		})
	}
}

func TestValidateGrammar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subject string
		body    string
		sugg    []proofread.Suggestion
		want    []string // kept quotes
	}{
		{
			name: "english and russian kept",
			body: "I has a cat. Он идёт домой и ашибка `code`.",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "has", Before: "I ", Replacement: "have"},
				{Field: "body", Quote: "идёт", Before: "Он ", Replacement: "идет"},
				{Field: "body", Quote: "ашибка", Replacement: "ошибка"},
				{Field: "body", Quote: "code", Replacement: "kod"},
			},
			want: []string{"body:has", "body:идёт", "body:ашибка"},
		},
		{
			name: "made up and no-op dropped",
			body: "I has a cat",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "dog", Replacement: "cat"},
				{Field: "body", Quote: "cat", Replacement: "cat"},
				{Field: "body", Quote: "has", Replacement: "have"},
			},
			want: []string{"body:has"},
		},
		{
			name: "overlap keeps the first",
			body: "Ths is bad texxt",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "Ths is", Replacement: "This is"},
				{Field: "body", Quote: "is bad", Replacement: "is poor"},
				{Field: "body", Quote: "texxt", Replacement: "text"},
			},
			want: []string{"body:Ths is", "body:texxt"},
		},
		{
			name:    "subject is its own field, not protected by body markdown",
			subject: "A `bad` titel",
			body:    "A `bad` titel in code, titel",
			sugg: []proofread.Suggestion{
				{Field: "subject", Quote: "titel", Replacement: "title"},
				{Field: "subject", Quote: "bad", Replacement: "good"},
				{Field: "body", Quote: "bad", Replacement: "good"},
			},
			want: []string{"subject:titel", "subject:bad"},
		},
		{
			name: "code span and block dropped",
			body: "Use `fmt.Prnt` and\n\n```\nx := tehh\n```\n\ntehh end",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "Prnt", Replacement: "Print"},
				{Field: "body", Quote: "tehh", Before: ":= ", Replacement: "the"},
				{Field: "body", Quote: "tehh", Before: "\n\n", Replacement: "the"},
			},
			want: []string{"body:tehh"},
		},
		{
			name: "link text allowed, url protected",
			body: "See [the dokument](https://example.com/dokument) now",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "dokument", Before: "the ", Replacement: "document"},
				{Field: "body", Quote: "dokument", Before: "com/", Replacement: "document"},
			},
			want: []string{"body:dokument"},
		},
		{
			name: "spoiler content allowed, tag line protected",
			body: "{spoiler Секрет}\nтекст с ашибкой\n{/spoiler}",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "ашибкой", Before: "с ", Replacement: "ошибкой"},
				{Field: "body", Quote: "Секрет", Replacement: "Тайна"},
			},
			want: []string{"body:ашибкой"},
		},
		{
			name: "raw html dropped",
			body: "a <span class=\"x\">tehh</span> tehh",
			sugg: []proofread.Suggestion{
				{Field: "body", Quote: "class", Replacement: "klass"},
				{Field: "body", Quote: "tehh", Before: "> ", Replacement: "the"},
			},
			want: []string{"body:tehh"},
		},
		{
			name: "unknown field dropped",
			body: "tehh",
			sugg: []proofread.Suggestion{{Field: "title", Quote: "tehh", Replacement: "the"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := proofread.ValidateGrammar(
				proofread.GrammarInput{Subject: tt.subject, Body: tt.body},
				proofread.GrammarOutput{Suggestions: tt.sugg})

			var quotes []string
			for _, c := range got {
				quotes = append(quotes, string(c.Field)+":"+c.Quote)
			}

			require.Equal(t, tt.want, quotes)
		})
	}
}

func TestValidateGrammarRepeatedMistake(t *testing.T) {
	t.Parallel()

	body := "Она одела платье. Потом она одела шляпу."
	got := proofread.ValidateGrammar(
		proofread.GrammarInput{Body: body},
		proofread.GrammarOutput{Suggestions: []proofread.Suggestion{
			{Field: "body", Quote: "одела", Replacement: "надела"},
			{Field: "body", Quote: "одела", Replacement: "надела"},
			{Field: "body", Quote: "одела", Replacement: "надела"},
		}})

	require.Len(t, got, 2)

	first, second := strings.Index(body, "одела"), strings.LastIndex(body, "одела")
	require.Equal(t, proofread.Match{Start: first, End: first + len("одела"), Needle: "одела", Occurrence: 0}, got[0].Match)
	require.Equal(t, proofread.Match{Start: second, End: second + len("одела"), Needle: "одела", Occurrence: 1}, got[1].Match)
}

func TestValidateStyle(t *testing.T) {
	t.Parallel()

	in := proofread.StyleInput{
		Subject: "Мой пост",
		Body:    "Первый абзац.\n\nТекст с `кодом` и [ссылкой](https://example.com).",
		Previous: &proofread.PreviousReview{
			Notes: []proofread.PreviousNote{{ID: "n1"}},
		},
	}

	out := proofread.StyleOutput{Notes: []proofread.Note{
		{Field: "none", Text: "general", Quote: "ignored", Continues: "n1"},
		{Field: "body", Text: "empty quote is general", Continues: "gone"},
		{Field: "subject", Quote: "пост", Text: "title"},
		{Field: "body", Quote: "Первый абзац.", Text: "ok", Continues: "n1"},
		{Field: "body", Quote: "Первый", Text: "overlaps the previous note"},
		{Field: "body", Quote: "кодом", Text: "in code"},
		{Field: "body", Quote: "ссылкой", Text: "link text"},
		{Field: "body", Quote: "example.com", Text: "url"},
		{Field: "body", Quote: "выдумано", Text: "made up"},
	}}

	got := proofread.ValidateStyle(in, out)

	type brief struct{ text, field, quote, continues string }

	var have []brief
	for _, n := range got {
		have = append(have, brief{n.Text, string(n.Field), n.Quote, n.Continues})
	}

	require.Equal(t, []brief{
		{"general", "none", "", "n1"},
		{"empty quote is general", "none", "", ""},
		{"title", "subject", "пост", ""},
		{"ok", "body", "Первый абзац.", "n1"},
		{"link text", "body", "ссылкой", ""},
	}, have)
}

func TestValidateStyleNoPrevious(t *testing.T) {
	t.Parallel()

	got := proofread.ValidateStyle(
		proofread.StyleInput{Body: "text"},
		proofread.StyleOutput{Notes: []proofread.Note{{Field: "none", Text: "x", Continues: "n1"}}})

	require.Len(t, got, 1)
	require.Empty(t, got[0].Continues)
}
