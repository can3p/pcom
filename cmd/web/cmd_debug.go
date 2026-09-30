package main

import (
	"errors"
	"fmt"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/jessevdk/go-flags"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

// addDebugCommands registers `debug feed`.
func addDebugCommands(p *flags.Parser) {
	debug, err := p.AddCommand("debug", "Developer tools", "Tools for looking at how pcom sees the outside world.", &struct{}{})
	if err != nil {
		panic(err)
	}

	mustAdd(debug.AddCommand("feed", "Show how a feed is read", "Fetch the feed at <url> and print every item raw, sanitized and as markdown.", &debugFeedCmd{}))
}

type debugFeedCmd struct {
	Args struct {
		URL string `positional-arg-name:"url" description:"Feed URL"`
	} `positional-args:"yes"`
}

func (c *debugFeedCmd) Execute([]string) error {
	if c.Args.URL == "" {
		return errors.New("usage: web debug feed <url>")
	}

	feed, err := gofeed.NewParser().ParseURL(c.Args.URL)
	if err != nil {
		return err
	}

	p := bluemonday.UGCPolicy()

	fmt.Println("title", feed.Title)
	fmt.Println("description", feed.Description)

	for idx, item := range feed.Items {
		fmt.Printf("Item %d:\n\n", idx+1)

		if item.PublishedParsed != nil {
			fmt.Printf("PublishedAt: %s\n", item.PublishedParsed)
		}

		fmt.Printf("Title: %s\n", item.Title)
		fmt.Printf("Raw Content: %s\n\n", item.Description)

		sanitized := p.Sanitize(item.Description)

		fmt.Printf("Sanitized Content: %s\n\n", sanitized)

		markdown, err := htmltomarkdown.ConvertString(sanitized)
		if err != nil {
			return err
		}

		fmt.Printf("MD Content: %s\n\n", markdown)
	}

	return nil
}
