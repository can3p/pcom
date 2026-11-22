package markdown

import (
	"github.com/can3p/pcom/pkg/markdown/mdext/figure"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// StandaloneImage is an image alone on its line, which the figure extension
// renders as a figure.
type StandaloneImage struct {
	// Markdown is the image as written, and Start and Stop its bytes in the
	// text.
	Markdown    string
	Destination string
	Start, Stop int
}

// StandaloneImages lists, in order, the images of md that stand alone on
// their line, by the figure extension's rule: an image in a line of text, in
// a link or in a code block is not one.
func StandaloneImages(md string) []StandaloneImage {
	source := []byte(md)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var out []StandaloneImage

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		p, ok := n.(*ast.Paragraph)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}

		for i, img := range figure.ImageLines(p, source) {
			if img == nil {
				continue
			}

			seg := p.Lines().At(i)
			seg = seg.TrimLeftSpace(source)
			seg = seg.TrimRightSpace(source)
			out = append(out, StandaloneImage{
				Markdown:    string(seg.Value(source)),
				Destination: string(img.Destination),
				Start:       seg.Start,
				Stop:        seg.Stop,
			})
		}

		return ast.WalkSkipChildren, nil
	})

	return out
}
