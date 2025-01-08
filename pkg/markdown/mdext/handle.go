package mdext

import (
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var usernameRE = regexp.MustCompile(`^[a-z][0-9a-z]*(_[0-9a-z]+)*`)

// An UserHandle struct represents an autolink of the Markdown text.
type UserHandle struct {
	ast.BaseInline

	value *ast.Text
}

// Inline implements Inline.Inline.
func (n *UserHandle) Inline() {}

// Dump implements Node.Dump
func (n *UserHandle) Dump(source []byte, level int) {
	segment := n.value.Segment
	m := map[string]string{
		"Value": string(segment.Value(source)),
	}
	ast.DumpHelper(n, source, level, m, nil)
}

// KindUserHandle is a NodeKind of the UserHandle node.
var KindUserHandle = ast.NewNodeKind("UserHandle")

// Kind implements Node.Kind.
func (n *UserHandle) Kind() ast.NodeKind {
	return KindUserHandle
}

func (n *UserHandle) UserName(source []byte) []byte {
	return n.value.Value(source)[1:]
}

func (n *UserHandle) Label(source []byte) []byte {
	return n.value.Value(source)
}

// NewUserHandle returns a new UserHandle node.
func NewUserHandle(value *ast.Text) *UserHandle {
	return &UserHandle{
		BaseInline: ast.BaseInline{},
		value:      value,
	}
}

type handleParser struct{}

// NewHandleParser return a new InlineParser can parse
// text that seems like a URL.
func NewHandleParser() parser.InlineParser {
	return &handleParser{}
}

func (s *handleParser) Trigger() []byte {
	// ' ' indicates any white spaces and a line head
	return []byte{'@'}
}

const (
	AutoLinkUserHandle ast.AutoLinkType = ast.AutoLinkURL + 1
)

func (s *handleParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if pc.IsInLinkLabel() {
		return nil
	}

	line, segment := block.PeekLine()
	consumes := 0
	start := segment.Start
	c := line[0]
	// advance if current position is not a line head.
	if c == '@' {
		consumes++
		line = line[1:]
	}

	l := usernameRE.Find(line)
	consumes += len(l)

	if len(l) < 3 {
		return nil
	}

	block.Advance(consumes)
	n := ast.NewTextSegment(text.NewSegment(start, start+consumes))
	link := NewUserHandle(n)
	return link
}

func (s *handleParser) CloseBlock(parent ast.Node, pc parser.Context) {
	// nothing to do
}

type handle struct{}

// NewHandle creates a new [goldmark.Extender] that
// allow you to parse text that seems like a @userhandle
func NewHandle() goldmark.Extender {
	return &handle{}
}

func (e *handle) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(NewHandleParser(), 999),
		),
	)
}
