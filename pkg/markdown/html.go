package markdown

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
)

func ToTemplate(s string) template.HTML {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(s), &buf); err != nil {
		panic(err)
	}

	return template.HTML(buf.String())
}
