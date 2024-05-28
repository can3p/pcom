package markdown

import (
	"bytes"
	"log"
	"strings"

	markdown "github.com/teekennedy/goldmark-markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type imgReplaceTransformer struct {
	Replacer Replacer
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

func ReplaceImageUrls(md string, replace Replacer) string {
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

func ImportReplacer(renameMap map[string]string, existingMap map[string]struct{}) Replacer {
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
