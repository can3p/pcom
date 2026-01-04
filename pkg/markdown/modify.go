package markdown

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/can3p/pcom/pkg/types"
	markdown "github.com/teekennedy/goldmark-markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type imgReplaceTransformer struct {
	Replacer types.Replacer[string]
}

func (t *imgReplaceTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	// Walk the AST in depth-first fashion and apply transformations
	err := ast.Walk(node, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		// Each node will be visited twice, once when it is first encountered (entering), and again
		// after all the node's children have been visited (if any). Skip the latter.
		if !entering {
			return ast.WalkContinue, nil
		}

		if node.Kind() == ast.KindImage {
			imgNode := node.(*ast.Image)

			shouldReplace, newValue := t.Replacer(string(imgNode.Destination))

			if shouldReplace {
				imgNode.Destination = []byte(newValue)
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		log.Fatal("Error encountered while transforming AST:", err)
	}
}

func NewModifier(t parser.ASTTransformer) goldmark.Markdown {
	// Goldmark supports multiple AST transformers and runs them sequentially in order of priority.
	prioritizedTransformer := util.Prioritized(t, 0)

	gm := goldmark.New(
		goldmark.WithRenderer(markdown.NewRenderer()),
		goldmark.WithParserOptions(parser.WithASTTransformers(prioritizedTransformer)),
	)

	return gm
}

func ReplaceImageUrls(md string, replace types.Replacer[string]) string {
	t := &imgReplaceTransformer{
		Replacer: replace,
	}

	gm := NewModifier(t) // Output buffer

	buf := bytes.Buffer{}

	// Convert parses the source, applies transformers, and renders output to the given io.Writer
	err := gm.Convert([]byte(md), &buf)
	if err != nil {
		log.Fatalf("Encountered Markdown conversion error: %v", err)
	}

	return strings.TrimSpace(buf.String())

}

type ErrorReplacer func(in string) (string, error)

type imgReplaceOrLinkifyTransformer struct {
	Replacer ErrorReplacer
}

func (t *imgReplaceOrLinkifyTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	type imageReplacement struct {
		img         *ast.Image
		parent      ast.Node
		replacement ast.Node
	}

	var replacements []imageReplacement

	err := ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if n.Kind() == ast.KindImage {
			imgNode := n.(*ast.Image)

			newValue, replaceErr := t.Replacer(string(imgNode.Destination))

			if replaceErr != nil {
				parent := imgNode.Parent()
				link := ast.NewLink()
				link.Destination = []byte(string(imgNode.Destination))

				imgCaption := string(imgNode.Title)
				if len(imgCaption) == 0 && imgNode.HasChildren() {
					if firstChild, ok := imgNode.FirstChild().(*ast.Text); ok {
						imgCaption = string(firstChild.Value(reader.Source()))
					}
				}
				if len(imgCaption) == 0 {
					imgCaption = string(imgNode.Destination)
				}
				link.AppendChild(link, ast.NewString([]byte(imgCaption+": "+replaceErr.Error())))

				replacements = append(replacements, imageReplacement{
					img:         imgNode,
					parent:      parent,
					replacement: link,
				})
			} else {
				imgNode.Destination = []byte(newValue)
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		log.Fatal("Error encountered while transforming AST:", err)
	}

	// Apply replacements
	for _, repl := range replacements {
		repl.parent.InsertBefore(repl.parent, repl.img, repl.replacement)
		repl.parent.RemoveChild(repl.parent, repl.img)
	}
}

func ExtractImageUrls(md string) []string {
	var urls []string

	source := []byte(md)
	reader := text.NewReader(source)
	doc := goldmark.DefaultParser().Parse(reader)

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if node.Kind() == ast.KindImage {
			imgNode := node.(*ast.Image)
			urls = append(urls, string(imgNode.Destination))
		}

		return ast.WalkContinue, nil
	})

	return urls
}

func ReplaceImageUrlsOrLinkify(md string, replace ErrorReplacer) (string, error) {
	t := &imgReplaceOrLinkifyTransformer{
		Replacer: replace,
	}

	gm := NewModifier(t)

	buf := bytes.Buffer{}

	err := gm.Convert([]byte(md), &buf)
	if err != nil {
		return "", fmt.Errorf("failed to process markdown: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}

func ImportReplacer(renameMap map[string]string, existingMap map[string]struct{}) types.Replacer[string] {
	return func(in string) (bool, string) {
		destRaw := in
		dest := strings.ToLower(destRaw)

		// all this dance is only to make sure wrong case does not fool us
		// to treat an existing image as a new one
		if _, ok := existingMap[dest]; ok && dest != destRaw {
			return true, dest
		} else if newName, ok := renameMap[dest]; ok {
			return true, newName
		}

		return false, in
	}
}
