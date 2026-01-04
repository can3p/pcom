# Agent Context for PCOM Project

## Frontend Architecture

### Technology Stack
- **htmx** - AJAX requests and page transitions
- **Stimulus.js** - JavaScript controllers
- **Go Templates** - Server-side HTML rendering
- **Bootstrap** - CSS framework

### htmx Configuration
Located in `/Users/dima/code/pcom/cmd/web/client/js/index.js`:
- `htmx.config.includeIndicatorStyles = false` - CSP compliance
- `htmx.config.allowScriptTags = false` - Security and Turbo-like behavior
- `hx-boost="true"` on `<body>` enables smooth page transitions
- `hx-ext="head-support"` auto-merges `<head>` elements during navigation
- `json-enc` extension for JSON payloads (sets `Content-Type: application/json`, stringifies parameters)

### Template Structure
- Each page includes `{{ template "header.html" . }}` (contains `<html>`, `<head>`, `<body>`, nav)
- Each page includes `{{ template "footer.html" . }}` (closing tags, scripts)

### CSRF Protection
- Token passed via `hx-headers='{"X-CSRFToken": "{{ .User.CSRFToken }}"}'` on `<body>` tag

### Action Controller Pattern
**Location**: `/Users/dima/code/pcom/cmd/web/client/js/controllers/action_controller.js`

Generic controller for server actions with confirmation dialogs:
- **Values**: `action`, `prompt`, `promptField`, `skipReload`
- **Behavior**: Calls `/controls/action/{action}` via `runAction()` with JSON payload
- **`connect()`**: Automatically adds `json-enc` extension to element
- **`skipReload`**: When `true`, skips page reload on success (allows htmx response headers to control behavior)

**runAction Implementation** (`pkg/web/client/js/lib.js`):
- Uses `htmx.ajax()` instead of `fetch()` to enable htmx response header interpretation
- Reads `hx-target` and `hx-swap` attributes from element
- Constructs URL as `/controls/action/{name}` and payload from element dataset
- Merges CSRF headers from `<body hx-headers>`
- Returns promise that resolves on success or rejects with error

**Usage Pattern**:
```html
<div id="item-{{ .ID }}">
  <button data-controller="action"
          data-action="action#run"
          data-action-action-value="delete_item"
          data-action-prompt-value="Confirm?"
          data-action-skip-reload-value="true"
          data-id="{{ .ID }}"
          hx-target="#item-{{ .ID }}"
          hx-swap="delete">Delete</button>
</div>
```

Server can control behavior via htmx response headers (`HX-Reswap`, `HX-Redirect`, etc.)

## RSS Feed Image Processing

### Architecture
Images from RSS feeds are automatically downloaded and hosted locally to avoid third-party dependencies.

### Budget Limits
- **Max images per feed item**: 20 (enforced in `pkg/feedops/reader/cleaner.go`)
- **Max size per image**: 10 MB (enforced in `pkg/feedops/reader/fetcher.go`)
- **Max total size per item**: 50 MB (tracked in `pkg/feedops/reader/cleaner.go`)
- **Individual image timeout**: 30 seconds (per image download)
- **Global timeout**: 2 minutes (for all images in one feed item)

### Implementation Flow
1. **`SaveFeedItem`** (`pkg/feedops/feeder/feeder.go`) - Creates global timeout context and upload function
2. **`CreateImageReplacer`** (`pkg/feedops/reader/cleaner.go`) - Extracts image URLs, enforces max images limit, handles errors
3. **`FetchMedia`** (`pkg/feedops/reader/fetcher.go`) - Downloads images with size/timeout limits, validates MIME types
4. **`HandleUpload`** (`pkg/media/upload.go`) - Stores images linked to RSS feed ID
5. **`ReplaceImageUrls`** (`pkg/markdown/modify.go`) - Replaces URLs in markdown AST

### Error Handling
Failed downloads are replaced with readable error messages in markdown:
- `_[Image download timed out: URL]_`
- `_[Image too large: URL]_`
- `_[Image limit exceeded (20 max): URL]_`
- `_[Image download failed: URL]_`

### Database
- `media_uploads` table supports either `user_id` OR `rss_feed_id` (mutual exclusivity enforced)
- Migration: `migrations/20251231005904-media_uploads_feeds.sql`

## Testing

### Testing Packages
- **github.com/ovechkin-dm/mockio/v2** - Mock library for Go without code generation
- **github.com/stretchr/testify** - Assertion and testing utilities (require, assert)
- **testcontainers/postgres** - PostgreSQL test container helper

### Test Container Usage
Located in `/Users/dima/code/pcom/testcontainers/postgres`:
- Provides `NewTestDB()` function that returns a `*TestDB` with a clean database instance
- Each test gets its own isolated database
- Migrations are automatically applied from `/Users/dima/code/pcom/migrations`
- Container is shared across tests in a package for efficiency
- Container cleanup happens automatically after tests complete (with 5-minute expiration as fallback)
- Use `defer testDB.Close()` to clean up the database after each test

### Test Factory Pattern
Located in `/Users/dima/code/pcom/pkg/feedops/testutil/factory.go`:
- Factory functions for creating test entities: `CreateUser`, `CreateRSSFeed`, `CreateRSSItem`, etc.
- Helper functions for retrieving entities: `GetRSSFeed`, `GetRSSItemsByFeed`, `GetUserFeedItemsByUser`
- All factory functions accept `context.Context` and `boil.ContextExecutor` for transaction support

### Mockio v2 Usage
```go
import . "github.com/ovechkin-dm/mockio/v2/mock"

func TestExample(t *testing.T) {
    ctrl := NewMockController(t)
    mockObj := Mock[MyInterface](ctrl)
    
    // Single return value
    WhenSingle(mockObj.Method(Any[string]())).ThenReturn("result")
    
    // Multiple return values
    WhenDouble(mockObj.Method(Any[string]())).ThenReturn("result", nil)
    
    // Dynamic answers
    WhenSingle(mockObj.Method(Any[string]())).ThenAnswer(func(args []any) string {
        return "dynamic result"
    })
}
```

## File Locations
- HTML Templates: `/Users/dima/code/pcom/cmd/web/client/html/`
- JavaScript: `/Users/dima/code/pcom/cmd/web/client/js/`
- Main JS entry: `/Users/dima/code/pcom/cmd/web/client/js/index.js`
