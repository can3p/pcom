package videoembed

import (
	"github.com/can3p/pcom/pkg/links/media"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type VideoEmbed struct {
	ast.BaseBlock
	media media.MediaLink
}

func (n *VideoEmbed) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindVideoEmbed = ast.NewNodeKind("VideoEmbed")

func (n *VideoEmbed) Kind() ast.NodeKind {
	return KindVideoEmbed
}

func NewVideoEmbed(media media.MediaLink) *VideoEmbed {
	return &VideoEmbed{
		BaseBlock: ast.BaseBlock{},
		media:     media,
	}
}

type videoEmbedTransfomer struct {
	parser media.MediaParser
}

func NewVideoEmbedTransformer(parser media.MediaParser) *videoEmbedTransfomer {
	return &videoEmbedTransfomer{
		parser: parser,
	}
}

func (p *videoEmbedTransfomer) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
	lines := node.Lines()

	if lines.Len() != 1 {
		return
	}

	line := lines.At(0)

	possibleUrl := util.TrimLeftSpace(util.TrimRightSpace(line.Value(reader.Source())))

	mediaLink := p.parser.Parse(string(possibleUrl))

	if mediaLink == nil {
		return
	}

	newNode := NewVideoEmbed(mediaLink)

	newNode.SetBlankPreviousLines(node.HasBlankPreviousLines())
	node.Parent().ReplaceChild(node.Parent(), node, newNode)

}
