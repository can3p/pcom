package translate

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// CheckShape reports, wrapping ErrShape, how got doesn't match source: a
// different number of segments, or a segment whose protected spans
// (<span class="notranslate" data-i="n">) differ from its source's, by
// data-i and content. Spans may move within a segment, as word order does.
func CheckShape(source, got []string) error {
	if len(got) != len(source) {
		return fmt.Errorf("%w: %d segments, want %d", ErrShape, len(got), len(source))
	}

	for i := range source {
		want, err := protectedSpans(source[i])
		if err != nil {
			return fmt.Errorf("%w: source segment %d: %w", ErrShape, i, err)
		}

		have, err := protectedSpans(got[i])
		if err != nil {
			return fmt.Errorf("%w: segment %d: %w", ErrShape, i, err)
		}

		for k, v := range want {
			if hv, ok := have[k]; !ok {
				return fmt.Errorf("%w: segment %d lost protected span data-i=%q", ErrShape, i, k)
			} else if hv != v {
				return fmt.Errorf("%w: segment %d changed protected span data-i=%q", ErrShape, i, k)
			}
		}

		if len(have) != len(want) {
			return fmt.Errorf("%w: segment %d has %d protected spans, want %d", ErrShape, i, len(have), len(want))
		}
	}

	return nil
}

// protectedSpans maps each notranslate span's data-i to its content,
// normalized by re-rendering its tokens so that quoting and escaping
// differences don't count.
func protectedSpans(fragment string) (map[string]string, error) {
	spans := map[string]string{}
	z := html.NewTokenizer(strings.NewReader(fragment))

	var (
		key   string
		buf   strings.Builder
		depth int // nesting of spans inside the open protected span; 0: none open
	)

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if depth > 0 {
				return nil, fmt.Errorf("protected span data-i=%q is not closed", key)
			}

			return spans, nil
		}

		tok := z.Token()

		if depth == 0 {
			if tt == html.StartTagToken && tok.Data == "span" && isProtected(tok) {
				key, depth = attr(tok, "data-i"), 1
				buf.Reset()

				if _, dup := spans[key]; dup {
					return nil, fmt.Errorf("protected span data-i=%q appears twice", key)
				}
			}

			continue
		}

		if tok.Data == "span" {
			switch tt { //nolint:exhaustive // only span boundaries change the depth
			case html.StartTagToken:
				depth++
			case html.EndTagToken:
				depth--
				if depth == 0 {
					spans[key] = buf.String()
					continue
				}
			}
		}

		buf.WriteString(tok.String())
	}
}

func isProtected(tok html.Token) bool {
	return slices.Contains(strings.Fields(attr(tok, "class")), "notranslate")
}

func attr(tok html.Token, name string) string {
	for _, a := range tok.Attr {
		if a.Key == name {
			return a.Val
		}
	}

	return ""
}
