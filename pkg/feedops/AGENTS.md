# RSS feed operations

Read this before changing feed fetching or image handling in `pkg/feedops/`.

## Architecture
Images from RSS feeds are automatically downloaded and hosted locally to avoid third-party dependencies.

## Budget Limits
- **Max images per feed item**: 20 (enforced in `pkg/feedops/reader/cleaner.go`)
- **Max size per image**: 10 MB (enforced in `pkg/feedops/reader/fetcher.go`)
- **Max total size per item**: 50 MB (tracked in `pkg/feedops/reader/cleaner.go`)
- **Individual image timeout**: 30 seconds (per image download)
- **Global timeout**: 2 minutes (for all images in one feed item)

## Implementation Flow
1. **`SaveFeedItem`** (`pkg/feedops/feeder/feeder.go`) - Creates global timeout context and upload function
2. **`CreateImageReplacer`** (`pkg/feedops/reader/cleaner.go`) - Extracts image URLs, enforces max images limit, handles errors
3. **`FetchMedia`** (`pkg/feedops/reader/fetcher.go`) - Downloads images with size/timeout limits, validates MIME types
4. **`HandleUpload`** (`pkg/media/upload.go`) - Stores images linked to RSS feed ID
5. **`ReplaceImageUrls`** (`pkg/markdown/modify.go`) - Replaces URLs in markdown AST

## Error Handling
Failed downloads are replaced with readable error messages in markdown:
- `_[Image download timed out: URL]_`
- `_[Image too large: URL]_`
- `_[Image limit exceeded (20 max): URL]_`
- `_[Image download failed: URL]_`

## Database
- `media_uploads` table supports either `user_id` OR `rss_feed_id` (mutual exclusivity enforced)
- Migration: `migrations/20251231005904-media_uploads_feeds.sql`
