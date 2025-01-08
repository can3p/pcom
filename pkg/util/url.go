package util

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// NormalizeURL normalizes a URL by:
// 1. Removing trailing slashes and question marks
// 2. Normalizing query parameter order
// Returns an error if URL is invalid
func NormalizeURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("URL cannot be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %w", err)
	}

	// If there's no scheme and no host, it's not a valid URL
	if u.Scheme == "" {
		return "", fmt.Errorf("URL must include a protocol (e.g., https://)")
	}
	if u.Host == "" {
		return "", fmt.Errorf("URL must include a domain name")
	}

	u.Host = strings.ToLower(u.Host)

	// Remove trailing slashes from path
	u.Path = strings.TrimRight(u.Path, "/")

	// Normalize query parameters
	q := u.Query()
	if len(q) > 0 {
		// Get sorted keys
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Build normalized query string
		newQuery := url.Values{}
		for _, k := range keys {
			values := q[k]
			sort.Strings(values) // Sort values for each key
			for _, v := range values {
				newQuery.Add(k, v)
			}
		}
		u.RawQuery = newQuery.Encode()
	}

	// Remove trailing question mark if no query parameters
	normalized := u.String()
	if u.RawQuery == "" {
		normalized = strings.TrimRight(normalized, "?")
	}

	return normalized, nil
}
