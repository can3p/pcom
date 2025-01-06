package postops

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
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

// StoreURL normalizes the given URL and stores it in the normalized_urls table,
// handling the case where the URL already exists using upsert.
// Returns the URL struct and any error.
func StoreURL(ctx context.Context, exec boil.ContextExecutor, rawURL string) (*core.NormalizedURL, error) {
	normalizedURL, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	// Generate UUID v7 for the new URL
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	urlID := id.String()

	// Create new URL record
	newURL := &core.NormalizedURL{
		ID:  urlID,
		URL: normalizedURL,
	}

	// Try to insert, if URL exists, get existing ID
	// we need to pass true to get existing id back
	err = newURL.Upsert(ctx, exec, true, []string{core.NormalizedURLColumns.URL}, boil.Infer(), boil.Infer())
	if err != nil {
		return nil, err
	}

	return newURL, nil
}
