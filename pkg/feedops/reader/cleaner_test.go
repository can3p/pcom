package reader_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/pkg/errors"
)

func TestCreateImageReplacer(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		inputMarkdown  string
		uploadResults  map[string]uploadResult
		expectedOutput string
	}{
		{
			name:          "single image success",
			inputMarkdown: "![alt text](https://example.com/image.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/image.jpg": {url: "/local/image1.jpg", err: nil},
			},
			expectedOutput: "![alt text](/local/image1.jpg)",
		},
		{
			name:          "multiple images success",
			inputMarkdown: "![img1](https://example.com/1.jpg)\n\n![img2](https://example.com/2.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/1.jpg": {url: "/local/1.jpg", err: nil},
				"https://example.com/2.jpg": {url: "/local/2.jpg", err: nil},
			},
			expectedOutput: "![img1](/local/1.jpg)\n\n![img2](/local/2.jpg)",
		},
		{
			name:          "image timeout error",
			inputMarkdown: "![timeout](https://example.com/slow.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/slow.jpg": {url: "", err: reader.ErrMediaTimeout},
			},
			expectedOutput: "*[Image download timed out: https://example.com/slow.jpg]*",
		},
		{
			name:          "image too large error",
			inputMarkdown: "![large](https://example.com/huge.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/huge.jpg": {url: "", err: reader.ErrMediaTooLarge},
			},
			expectedOutput: "*[Image too large: https://example.com/huge.jpg]*",
		},
		{
			name:          "generic download error",
			inputMarkdown: "![failed](https://example.com/broken.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/broken.jpg": {url: "", err: errors.New("network error")},
			},
			expectedOutput: "*[Image download failed: https://example.com/broken.jpg]*",
		},
		{
			name:          "mixed success and errors",
			inputMarkdown: "![ok](https://example.com/ok.jpg)\n![timeout](https://example.com/slow.jpg)\n![large](https://example.com/huge.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/ok.jpg":   {url: "/local/ok.jpg", err: nil},
				"https://example.com/slow.jpg": {url: "", err: reader.ErrMediaTimeout},
				"https://example.com/huge.jpg": {url: "", err: reader.ErrMediaTooLarge},
			},
			expectedOutput: "![ok](/local/ok.jpg)\n*[Image download timed out: https://example.com/slow.jpg]*\n*[Image too large: https://example.com/huge.jpg]*",
		},
		{
			name:          "max images limit",
			inputMarkdown: generateMarkdownWithImages(25),
			uploadResults: generateUploadResults(25),
			expectedOutput: func() string {
				var parts []string
				for i := 0; i < reader.MaxImagesPerFeedItem-1; i++ {
					parts = append(parts, "![img]("+generateLocalURL(i)+")")
				}
				for i := reader.MaxImagesPerFeedItem - 1; i < 25; i++ {
					parts = append(parts, "*[Image limit exceeded ("+fmt.Sprintf("%d", reader.MaxImagesPerFeedItem)+" max): "+generateImageURL(i)+"]*")
				}
				return strings.Join(parts, "\n\n")
			}(),
		},
		{
			name:           "no images",
			inputMarkdown:  "Just some text without images",
			uploadResults:  map[string]uploadResult{},
			expectedOutput: "Just some text without images",
		},
		{
			name:          "duplicate image URLs",
			inputMarkdown: "![img1](https://example.com/same.jpg)\n\n![img2](https://example.com/same.jpg)",
			uploadResults: map[string]uploadResult{
				"https://example.com/same.jpg": {url: "/local/same.jpg", err: nil},
			},
			expectedOutput: "![img1](/local/same.jpg)\n\n![img2](/local/same.jpg)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploadFunc := func(ctx context.Context, url string) (string, error) {
				result, ok := tt.uploadResults[url]
				if !ok {
					return "", errors.New("unexpected URL: " + url)
				}
				return result.url, result.err
			}

			replacer := reader.CreateImageReplacer(ctx, tt.inputMarkdown, nil, uploadFunc)
			output := markdown.ReplaceImageUrlsOrErrorMessage(tt.inputMarkdown, replacer)

			if output != tt.expectedOutput {
				t.Errorf("CreateImageReplacer() output mismatch\nGot:\n%s\n\nWant:\n%s", output, tt.expectedOutput)
			}
		})
	}
}

type uploadResult struct {
	url string
	err error
}

func generateMarkdownWithImages(count int) string {
	var parts []string
	for i := 0; i < count; i++ {
		parts = append(parts, "![img]("+generateImageURL(i)+")")
	}
	return strings.Join(parts, "\n\n")
}

func generateUploadResults(count int) map[string]uploadResult {
	results := make(map[string]uploadResult)
	for i := 0; i < count; i++ {
		url := generateImageURL(i)
		if i < reader.MaxImagesPerFeedItem-1 {
			results[url] = uploadResult{url: generateLocalURL(i), err: nil}
		}
	}
	return results
}

func generateImageURL(i int) string {
	return "https://example.com/image" + string(rune('0'+i)) + ".jpg"
}

func generateLocalURL(i int) string {
	return "/local/image" + string(rune('0'+i)) + ".jpg"
}
