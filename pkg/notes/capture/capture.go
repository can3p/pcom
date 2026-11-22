// Package capture reads what the owner put into the capture box: typed text,
// text shared into the app, or text sent by the quote shortcut. It decides
// what kind of note the text makes and writes the attribution line of a
// quote. The text is kept whole: only its outer whitespace goes.
package capture

import (
	"net/url"
	"regexp"
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

// Attribution is the line under a quote from the source titled title:
// "— *Title*", with the title's whitespace collapsed; "" for no title.
func Attribution(title string) string {
	t := strings.Join(strings.Fields(title), " ")
	if t == "" {
		return ""
	}

	return "— *" + t + "*"
}

// photoLine is a paragraph that is one image and nothing else: the form
// WithPhotos writes a photo in, and the editor's toolbar too.
var photoLine = regexp.MustCompile(`^!\[[^\]\n]*\]\([^)\s]+\)$`)

// WithPhotos is the body of a note captured with photos: each photo, an
// upload's file name as the markdown renderer serves it, as an image paragraph
// of its own in the order given, then text. The photos come first so the text
// reads as their caption, as it does in a post.
func WithPhotos(text string, fnames []string) string {
	parts := make([]string, 0, len(fnames)+1)
	for _, f := range fnames {
		parts = append(parts, "![]("+f+")")
	}

	if text = strings.TrimSpace(text); text != "" {
		parts = append(parts, text)
	}

	return strings.Join(parts, "\n\n")
}

// SplitPhotos separates a body into its photos, the lines that are an image
// alone, and the rest of the text with the gaps they leave closed.
func SplitPhotos(body string) (photos []string, text string) {
	var rest []string

	for l := range strings.SplitSeq(body, "\n") {
		if photoLine.MatchString(strings.TrimSpace(l)) {
			photos = append(photos, strings.TrimSpace(l))
		} else {
			rest = append(rest, l)
		}
	}

	text = strings.Join(rest, "\n")
	text = blankRun.ReplaceAllString(strings.TrimSpace(text), "\n\n")

	return photos, text
}

var blankRun = regexp.MustCompile(`\n{3,}`)
