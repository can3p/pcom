package reader

import (
	"html"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/microcosm-cc/bluemonday"
)

type Cleaner struct{}

func DefaultCleaner() *Cleaner {
	return &Cleaner{}
}

func (c *Cleaner) CleanField(in string) string {
	p := bluemonday.StrictPolicy()

	sanitized := p.Sanitize(in)

	return html.UnescapeString(sanitized)
}

func (c *Cleaner) HTMLToMarkdown(in string) (string, error) {
	p := bluemonday.UGCPolicy()

	sanitized := p.Sanitize(in)

	return htmltomarkdown.ConvertString(sanitized)
}
