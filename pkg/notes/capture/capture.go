// Package capture reads what the owner put into the capture box: typed text,
// text shared into the app, or text sent by the quote shortcut. It decides
// what kind of note the text makes and writes the attribution line of a
// quote. The text is kept whole: only its outer whitespace goes.
package capture

import (
	"net/url"
	"strings"
	"unicode"
)

// How the text reached the capture box.
type How int

// The values of How.
const (
	// Typed is text the owner typed or pasted.
	Typed How = iota
	// Shared is text shared into the app from another one.
	Shared
	// QuoteShortcut is text sent by the quote shortcut: always a quote.
	QuoteShortcut
)

// Kind is what a capture makes.
type Kind int

// The values of Kind.
const (
	// Plain is a note of text.
	Plain Kind = iota
	// URL is a note that is a single link.
	URL
	// Quote is text taken from a source.
	Quote
)

// Capture is the parsed text.
type Capture struct {
	Kind Kind
	// Text is the body to store: the input without its outer whitespace,
	// byte for byte otherwise. Shared and quoted text is a markdown
	// blockquote of it (see Unquote); a link and typed text stay as they
	// are.
	Text string
}

// Parse reads text captured in the way how. A text that is a single http(s)
// link is a URL note, as it is. Otherwise text sent by the quote shortcut is
// a quote and shared text is plain, both stored as a blockquote so the owner
// can tell the passage from their own words; typed text is plain and stored
// as typed. A link inside other text is no link.
func Parse(text string, how How) Capture {
	text = strings.TrimSpace(text)

	switch {
	case isURL(text):
		return Capture{Kind: URL, Text: text}
	case how == QuoteShortcut:
		return Capture{Kind: Quote, Text: blockquote(text)}
	case how == Shared:
		return Capture{Kind: Plain, Text: blockquote(text)}
	default:
		return Capture{Kind: Plain, Text: text}
	}
}

// blockquote prefixes every line with "> ", and a blank line with ">".
func blockquote(text string) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}

	return strings.Join(lines, "\n")
}

// Unquote is the passage a blockquote made by Parse holds: the "> " prefix
// of every line, or the ">" of a blank one, stripped.
func Unquote(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if l == ">" {
			lines[i] = ""
		} else {
			lines[i] = strings.TrimPrefix(l, "> ")
		}
	}

	return strings.Join(lines, "\n")
}

func isURL(s string) bool {
	if s == "" || strings.ContainsFunc(s, unicode.IsSpace) {
		return false
	}

	u, err := url.Parse(s)

	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Source is what the attribution line of a quote says; every part is
// optional.
type Source struct {
	Author string
	Title  string
	// Page is the page or locator as the owner wrote it, "12" or "12-14".
	Page string
}

// Attribution is the line under a quote: "— Author, *Title*, p. 12". Parts
// that are unknown are left out, with their separators; no part is "".
func Attribution(s Source) string {
	var parts []string

	if a := strings.Join(strings.Fields(s.Author), " "); a != "" {
		parts = append(parts, a)
	}

	if t := strings.Join(strings.Fields(s.Title), " "); t != "" {
		parts = append(parts, "*"+t+"*")
	}

	if p := strings.Join(strings.Fields(s.Page), " "); p != "" {
		parts = append(parts, "p. "+p)
	}

	if len(parts) == 0 {
		return ""
	}

	return "— " + strings.Join(parts, ", ")
}
