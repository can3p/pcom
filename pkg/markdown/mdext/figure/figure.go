// Package figure turns images with captions into <figure> elements.
//
// Outside a {gallery}, an image alone on its line becomes a figure: the lines
// after it in the same paragraph, up to the next image alone on its line, are
// its caption. Lines before it stay a paragraph, and so does a paragraph after
// a blank line. An image in a line of text stays inline.
//
// Inside a {gallery} every image becomes a figure, and everything between it
// and the next image (the rest of its line, the lines after it and the blocks
// after it) is its caption. Text before the first image is left where it is;
// the gallery controller shows it above the gallery.
package figure

import (
	"bytes"

	"github.com/can3p/pcom/pkg/markdown/mdext/blocktags"
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// KindFigure is the kind of a Figure node.
var KindFigure = ast.NewNodeKind("Figure")

// Figure holds an image and, optionally, a Caption after it.
type Figure struct {
	ast.BaseBlock
	caption *Caption
}

// Kind implements ast.Node.
func (n *Figure) Kind() ast.NodeKind { return KindFigure }

// Dump implements ast.Node.
func (n *Figure) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// Caption returns the figure's caption, creating it on first use.
func (n *Figure) Caption() *Caption {
	if n.caption == nil {
		n.caption = &Caption{}
		n.AppendChild(n, n.caption)
	}

	return n.caption
}

// KindCaption is the kind of a Caption node.
var KindCaption = ast.NewNodeKind("FigureCaption")

// Caption holds the blocks of a figure's caption.
type Caption struct {
	ast.BaseBlock
}

// Kind implements ast.Node.
func (n *Caption) Kind() ast.NodeKind { return KindCaption }

// Dump implements ast.Node.
func (n *Caption) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type extender struct {
	view types.HTMLView
}

// New returns the extension that builds and renders figures. Mails and RSS
// get them without classes: mail.css styles the elements themselves.
func New(view types.HTMLView) goldmark.Extender { return &extender{view: view} }

func (e *extender) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(&transformer{}, 500)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&figureRenderer{plain: e.view == types.ViewEmail || e.view == types.ViewRSS}, 500)))
}

type transformer struct{}

func (t *transformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	transformBlocks(doc, reader.Source())
}

func transformBlocks(parent ast.Node, source []byte) {
	for c := parent.FirstChild(); c != nil; {
		next := c.NextSibling()

		switch n := c.(type) {
		case *ast.Paragraph:
			if lines := splitLines(n); hasImageLine(lines, source) {
				splitParagraph(n, lines, false, nil, source)
			}
		case *blocktags.BlockTag:
			if n.BlockTagName == "gallery" {
				transformGallery(n, source)
			} else {
				transformBlocks(n, source)
			}
		default:
			if c.Type() == ast.TypeBlock {
				transformBlocks(c, source)
			}
		}

		c = next
	}
}

func transformGallery(gallery ast.Node, source []byte) {
	var fig *Figure

	for c := gallery.FirstChild(); c != nil; {
		next := c.NextSibling()

		switch {
		case isParagraphWithImage(c):
			p := c.(*ast.Paragraph)
			fig = splitParagraph(p, splitLines(p), true, fig, source)
		case containsImage(c):
			// images nested in other blocks, a list of images for example,
			// are left to the gallery controller as they are
			fig = nil
		case fig != nil:
			fig.Caption().AppendChild(fig.Caption(), c)
		}

		c = next
	}
}

// splitParagraph replaces p with figures and caption paragraphs. In a gallery
// every image starts a figure, elsewhere only an image alone on its line
// does. Text before the first figure goes into the caption of current, the
// figure before p, or stays as a paragraph when there is none. It returns the
// last figure.
func splitParagraph(p *ast.Paragraph, lines [][]ast.Node, gallery bool, current *Figure, source []byte) *Figure {
	parent := p.Parent()
	fig := current

	var pending []ast.Node

	flush := func() {
		nodes := trimBlank(pending, source)
		pending = nil

		if len(nodes) == 0 {
			return
		}

		para := ast.NewParagraph()
		para.SetLines(p.Lines())

		for _, n := range nodes {
			para.AppendChild(para, n)
		}

		if last, ok := nodes[len(nodes)-1].(*ast.Text); ok {
			last.SetSoftLineBreak(false)
			last.SetHardLineBreak(false)
		}

		if fig != nil {
			fig.Caption().AppendChild(fig.Caption(), para)
		} else {
			parent.InsertBefore(parent, p, para)
		}
	}

	startFigure := func(img *ast.Image) {
		flush()

		fig = &Figure{}
		fig.SetLines(p.Lines())
		fig.AppendChild(fig, img)
		parent.InsertBefore(parent, p, fig)
	}

	for _, line := range lines {
		if !gallery {
			if img := imageLine(line, source); img != nil {
				startFigure(img)
				continue
			}

			pending = append(pending, line...)

			continue
		}

		for _, n := range line {
			if img, ok := n.(*ast.Image); ok {
				startFigure(img)
				continue
			}

			pending = append(pending, n)
		}
	}

	flush()
	parent.RemoveChild(parent, p)

	return fig
}

// splitLines groups the inline children of p by source line. A line ends with
// the text node that carries its line break.
func splitLines(p ast.Node) [][]ast.Node {
	var (
		lines [][]ast.Node
		line  []ast.Node
	)

	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		line = append(line, c)

		if t, ok := c.(*ast.Text); ok && (t.SoftLineBreak() || t.HardLineBreak()) {
			lines = append(lines, line)
			line = nil
		}
	}

	if len(line) > 0 {
		lines = append(lines, line)
	}

	return lines
}

func hasImageLine(lines [][]ast.Node, source []byte) bool {
	for _, line := range lines {
		if imageLine(line, source) != nil {
			return true
		}
	}

	return false
}

// imageLine returns the image of a line that holds nothing but one image.
func imageLine(line []ast.Node, source []byte) *ast.Image {
	var img *ast.Image

	for _, n := range line {
		if isBlank(n, source) {
			continue
		}

		i, ok := n.(*ast.Image)
		if !ok || img != nil {
			return nil
		}

		img = i
	}

	return img
}

func trimBlank(nodes []ast.Node, source []byte) []ast.Node {
	for len(nodes) > 0 && isBlank(nodes[0], source) {
		nodes = nodes[1:]
	}

	for len(nodes) > 0 && isBlank(nodes[len(nodes)-1], source) {
		nodes = nodes[:len(nodes)-1]
	}

	return nodes
}

func isBlank(n ast.Node, source []byte) bool {
	t, ok := n.(*ast.Text)

	return ok && len(bytes.TrimSpace(t.Segment.Value(source))) == 0
}

func isParagraphWithImage(n ast.Node) bool {
	if _, ok := n.(*ast.Paragraph); !ok {
		return false
	}

	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.Image); ok {
			return true
		}
	}

	return false
}

func containsImage(n ast.Node) bool {
	found := false

	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := c.(*ast.Image); ok && entering {
			found = true
			return ast.WalkStop, nil
		}

		return ast.WalkContinue, nil
	})

	return found
}

type figureRenderer struct {
	plain bool
}

func (r *figureRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindFigure, r.renderFigure)
	reg.Register(KindCaption, r.renderCaption)
}

func (r *figureRenderer) renderFigure(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		if r.plain {
			_, _ = w.WriteString("<figure>\n")
		} else {
			_, _ = w.WriteString(`<figure class="post-figure">` + "\n")
		}
	} else {
		_, _ = w.WriteString("</figure>\n")
	}

	return ast.WalkContinue, nil
}

func (r *figureRenderer) renderCaption(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		if r.plain {
			_, _ = w.WriteString("\n<figcaption>\n")
		} else {
			_, _ = w.WriteString("\n" + `<figcaption class="post-figure__caption">` + "\n")
		}
	} else {
		_, _ = w.WriteString("</figcaption>\n")
	}

	return ast.WalkContinue, nil
}

var _ renderer.NodeRenderer = (*figureRenderer)(nil)
