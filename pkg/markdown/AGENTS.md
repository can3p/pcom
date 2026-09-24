# Markdown rendering

Read this before changing `pkg/markdown/` or the `markdown_*` template functions.

## Architecture
- **Library**: goldmark (extensible markdown parser)
- **Location**: `pkg/markdown/`
- **Entry point**: `ToEnrichedTemplate()` in `pkg/markdown/html.go`

## View Types
Defined in `pkg/types/types.go`:
- `ViewFeed` - RSS feed items (links open in new tab)
- `ViewSinglePost` - Individual post view
- `ViewEditPreview` - Post editor preview
- `ViewComment` - Comment rendering
- `ViewArticle` - Article view
- `ViewEmail` - Email notifications
- `ViewRSS` - RSS feed output

## Template Functions
Defined in `cmd/web/main.go` funcmap:
- `markdown_feed` - Renders feed items with `ViewFeed`
- `markdown_single_post` - Renders posts with `ViewSinglePost`
- `markdown_edit_preview` - Renders editor preview with `ViewEditPreview`
- `markdown_comment` - Renders comments with `ViewComment`
- `markdown_article` - Renders articles with `ViewArticle`

## Custom Renderers
Located in `pkg/markdown/mdext/`:
- **linkrenderer** - Custom link renderer, adds `target="_blank" rel="noopener noreferrer"` for `ViewFeed`
- **lazyload** - Lazy loading for images
- **blocktags** - Custom block tags (gallery, etc.)
- **headershift** - Shifts header levels (h1→h2, etc.)
- **videoembed** - Embeds videos from URLs
- **handle** - Renders @username mentions

## Extensions
Configured in `NewParser()`:
- **Linkify** - Auto-converts URLs to links (custom regex excludes closing braces)
- **Syntax highlighting** - Only for feed, single post, and edit preview views
- Custom node renderers registered via `util.Prioritized()` with priority 500

## Adding Custom Renderers
1. Create renderer in `pkg/markdown/mdext/<name>/`
2. Implement `renderer.NodeRenderer` interface with `RegisterFuncs()`
3. Register in `NewParser()` via `nodeRenderers` slice
4. Conditionally add based on view type if needed
