package postops

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// StoreURL normalizes the given URL and stores it in the normalized_urls table,
// handling the case where the URL already exists using upsert.
// Returns the URL struct and any error.
func StoreURL(ctx context.Context, exec boil.ContextExecutor, rawURL string) (*core.NormalizedURL, error) {
	normalizedURL, err := util.NormalizeURL(rawURL)
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
