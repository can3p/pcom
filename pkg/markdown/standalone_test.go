package markdown_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/stretchr/testify/require"
)

func TestStandaloneImages(t *testing.T) {
	t.Parallel()

	md := "intro\n![](a.jpg)\ncaption\n\n> ![q](b.png)\n\n- ![](c.jpg)\n\ntext ![](d.jpg)\n\n```\n![](e.jpg)\n```\n\n  ![t](f.jpg \"title\")  "
	// an item of a tight list is no paragraph, so the renderer makes no figure of it either
	got := markdown.StandaloneImages(md)

	want := []markdown.StandaloneImage{
		{Markdown: "![](a.jpg)", Destination: "a.jpg"},
		{Markdown: "![q](b.png)", Destination: "b.png"},
		{Markdown: `![t](f.jpg "title")`, Destination: "f.jpg"},
	}
	require.Len(t, got, len(want))

	for i, w := range want {
		require.Equal(t, w.Markdown, got[i].Markdown)
		require.Equal(t, w.Destination, got[i].Destination)
		require.Equal(t, w.Markdown, md[got[i].Start:got[i].Stop], "Start and Stop are its bytes")
	}

	require.Empty(t, markdown.StandaloneImages(""))
}
