package blocktags

import (
	"github.com/can3p/pcom/pkg/types"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type BlockTagRenderer struct {
	html.Config
	view types.HTMLView
	link types.Link
}

func NewBlockTagRenderer(view types.HTMLView, link types.Link, opts ...html.Option) renderer.NodeRenderer {
	r := &BlockTagRenderer{
		Config: html.NewConfig(),
		view:   view,
		link:   link,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *BlockTagRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindBlockTag, r.renderBlockTag)
}

func (r *BlockTagRenderer) renderBlockTag(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*BlockTag)
	title := n.BlockTitle

	if title == "" {
		switch n.BlockTagName {
		case "cut":
			title = "See more"
		case "spoiler":
			title = "Open Spoiler"

		}
	}

	if n.BlockTagName == "cut" && r.view == types.ViewFeed {
		if entering {
			// single_post_special is just a dummy name that
			// allows us to handle a special case.
			// Unfortunately the goldmark api does not allow us
			// to pass any options to renderer for a particular
			// text beign rendered. See the ugly hack in funcmap definition
			_, _ = w.WriteString(`<div class="post-cut-link"><a href="` + r.link("single_post_special") + `">`)
			_, _ = w.WriteString(title)
			return ast.WalkSkipChildren, nil
		} else {
			_, _ = w.WriteString("</a></div>\n")
			return ast.WalkContinue, nil
		}
	}

	// cut disappears in the full post
	if n.BlockTagName == "cut" && r.view == types.ViewSinglePost {
		return ast.WalkContinue, nil
	}

	if entering {
		var add string

		if r.view == types.ViewEditPreview {
			add = "edit-preview-"
		}

		_, _ = w.WriteString(`<div class="block-container-` + add + n.BlockTagName + `"`)
		if n.BlockTagName == "spoiler" && r.view != types.ViewEditPreview {
			_, _ = w.WriteString(` data-controller="spoiler"`)
		}

		if n.BlockTagName == "gallery" {
			_, _ = w.WriteString(` data-controller="gallery"`)
		}

		_, _ = w.WriteString(">\n")

		if title != "" {
			_, _ = w.WriteString(`<div class="block-container-` + add + n.BlockTagName + `-summary">` + string(util.EscapeHTML([]byte(title))) + "</div>\n")
		}

		_, _ = w.WriteString(`<div class="block-container-` + add + n.BlockTagName + `-content">` + "\n")
	} else {
		_, _ = w.WriteString("</div>\n")
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}
