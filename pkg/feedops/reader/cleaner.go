package reader

import (
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/microcosm-cc/bluemonday"
)

type Cleaner struct{}

func DefaultCleaner() *Cleaner {
	return &Cleaner{}
}

func (c *Cleaner) CleanField(in string) string {
	p := bluemonday.StrictPolicy()

	return p.Sanitize(in)
}

func (c *Cleaner) HTMLToMarkdown(in string) (string, error) {
	p := bluemonday.UGCPolicy()

	sanitized := p.Sanitize(in)

	return htmltomarkdown.ConvertString(sanitized)
}
