package media

import "html/template"

type MediaLink interface {
	EmbedCode() template.HTML
	Key() string
	URL() string
}

type MediaLinkSlice []MediaLink

func (cl MediaLinkSlice) Deduplicate() MediaLinkSlice {
	cache := map[string]struct{}{}
	out := []MediaLink{}

	for _, l := range cl {
		if _, ok := cache[l.Key()]; ok {
			continue
		}

		cache[l.Key()] = struct{}{}
		out = append(out, l)
	}

	return out
}

type MediaParser interface {
	Parse(url string) MediaLink
}

type AggregateParser struct {
	Parsers []MediaParser
}

func (p *AggregateParser) Parse(url string) MediaLink {
	for _, p := range p.Parsers {
		if out := p.Parse(url); out != nil {
			return out
		}
	}

	return nil
}

func DefaultParser() MediaParser {
	return &AggregateParser{
		Parsers: []MediaParser{
			&YoutubeParser{},
		},
	}
}
