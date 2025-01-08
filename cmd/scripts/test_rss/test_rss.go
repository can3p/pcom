package main

import (
	"fmt"
	"log"
	"os"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

func main() { //nolint:typecheck
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run cmd/scripts/test_rss.go <url>")
	}
	rssURL := os.Args[1]

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(rssURL)

	if err != nil {
		panic(err)
	}

	p := bluemonday.UGCPolicy()

	fmt.Println("title", feed.Title)
	fmt.Println("description", feed.Description)

	for idx, item := range feed.Items {
		title := item.Title
		content := item.Description
		publishedAt := item.PublishedParsed

		fmt.Printf("Item %d:\n\n", idx+1)

		if publishedAt != nil {
			fmt.Printf("PublishedAt: %s\n", publishedAt)
		}

		fmt.Printf("Title: %s\n", title)
		fmt.Printf("Raw Content: %s\n\n", content)

		sanitized := p.Sanitize(content)

		fmt.Printf("Sanitized Content: %s\n\n", sanitized)

		markdown, err := htmltomarkdown.ConvertString(sanitized)
		if err != nil {
			panic(err)
		}

		fmt.Printf("MD Content: %s\n\n", markdown)
	}
}
