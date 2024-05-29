package blocktags

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var OpeningBlockTagRe = regexp.MustCompile(`^{([a-z]+)}`)
var ClosingBlockTagRe = regexp.MustCompile(`^{/([a-z]+)}`)

type BlockTag struct {
	ast.BaseBlock
	BlockTagName string
}

func (n *BlockTag) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindBlockTag = ast.NewNodeKind("BlockTag")

func (n *BlockTag) Kind() ast.NodeKind {
	return KindBlockTag
}

func NewBlockTag(name string) *BlockTag {
	return &BlockTag{
		BaseBlock:    ast.BaseBlock{},
		BlockTagName: name,
	}
}

type TagDef struct {
	Name              string
	AllowedParentTags []string
}

var DefaultTags = []*TagDef{
	{
		Name:              "cut",
		AllowedParentTags: nil,
	},
	{
		Name:              "spoiler",
		AllowedParentTags: []string{"cut"},
	},
	{
		Name:              "gallery",
		AllowedParentTags: []string{"spoiler", "cut"},
	},
}

type DefMap map[string]*TagDef

func (dm DefMap) GetTag(name string) *TagDef {
	return dm[name]
}

type blockTagParser struct {
	AllowedTags DefMap
}

func NewBlockTagParser(allowed []*TagDef) *blockTagParser {
	return &blockTagParser{
		AllowedTags: lo.Associate(allowed, func(in *TagDef) (string, *TagDef) {
			return in.Name, in
		}),
	}
}

func (p *blockTagParser) Trigger() []byte {
	return []byte{'{'}
}

func (p *blockTagParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	fmt.Println("1")

	line = line[pos:]

	groups := OpeningBlockTagRe.FindSubmatch(line)

	if groups == nil {
		fmt.Println("2")
		return nil, parser.NoChildren
	}

	name := strings.ToLower(string(groups[1]))

	tagDef := p.AllowedTags.GetTag(name)

	if tagDef == nil {
		fmt.Println("3")
		return nil, parser.NoChildren
	}

	if util.TrimRightSpaceLength(line) != 1 {
		fmt.Printf("4 [%s] [%s], %d v %d\n", string(line), string(groups[0]), util.TrimRightSpaceLength(line), len(groups[0]))
		return nil, parser.NoChildren
	}

	parents := pc.OpenedBlocks()

	for i := len(parents) - 1; i >= 0; i-- {
		if b, ok := parents[i].Node.(*BlockTag); ok {
			if !lo.Contains(tagDef.AllowedParentTags, b.BlockTagName) {
				// if a tag is forbidden, just eat it
				// XXX: we can generte some debug comment there
				reader.Advance(len(line))
				return nil, parser.NoChildren
			}

			break
		}
	}

	node := NewBlockTag(name)
	reader.Advance(len(line))

	return node, parser.HasChildren
}

func (p *blockTagParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n := node.(*BlockTag)
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	line = line[pos:]

	groups := ClosingBlockTagRe.FindSubmatch(line)

	if groups == nil {
		return parser.Continue | parser.HasChildren
	}

	// simplification: do not deal with situations when there is any text after the block tag
	// XXX: better to allow goldmark to create the next block element out of this
	if util.TrimRightSpaceLength(line) != 1 {
		return parser.Continue | parser.HasChildren
	}

	name := strings.ToLower(string(groups[1]))

	if name == n.BlockTagName {
		reader.Advance(len(line))
		return parser.Close
	}

	return parser.Continue | parser.HasChildren
}

func (p *blockTagParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
}

func (p *blockTagParser) CanInterruptParagraph() bool {
	return true
}

func (p *blockTagParser) CanAcceptIndentedLine() bool {
	return false
}
