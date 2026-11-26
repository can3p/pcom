# RSS feed operations

Read this before changing feed fetching or image handling in `pkg/feedops/`.

## Architecture
Images from RSS feeds are automatically downloaded and hosted locally to avoid third-party dependencies.

## Budget Limits
- **Max images per feed item**: 20 (enforced in `pkg/feedops/reader/cleaner.go`)
- **Max size per image**: 10 MB (enforced in `pkg/feedops/reader/fetcher.go`). A larger body fails with
  `ErrMediaTooLarge`, whether or not it sends a `Content-Length`; it is never cut short.
- **Feed document**: at most `--rss-max-feed-size` bytes (5 MiB) within `--rss-fetch-timeout` (5s).
- **Individual image timeout**: `--rss-image-timeout` (30s per image download)
- **Global timeout**: 2 minutes (for all images in one feed item)

The fetcher's HTTP client has no `Timeout` of its own: every request gets its deadline from the context, so
the feed timeout and the image timeout stay independent. A client-wide timeout would cut every image download
short at the feed timeout.

## Public addresses only
`DefaultFetcher` connects only to public addresses (`publicOnly` in `pkg/feedops/reader/guard.go`, a
`net.Dialer` Control). The check runs on the address being dialed, after DNS and for every redirect hop, so
neither a redirect nor a name that resolves to a private address gets past it; it fails with
`ErrPrivateAddress`. The guarded client dials directly, ignoring `HTTPS_PROXY`: through a proxy the dialed
address would be the proxy's. `Limits.AllowPrivateAddresses` (`--rss-allow-private-addresses`) turns the guard
off; the e2e harness sets it because its feeds are served on 127.0.0.1, and service tests that fetch from an
`httptest` server pass it through `feeds.WithFetchLimits`. Tests built on `reader.NewFetcher` pass their own
client and are not guarded.

## Subscribing
`Subscribe` (`pkg/service/feeds/feeds.go`) takes a feed or a website address. A URL already stored as a feed
is subscribed to without a download; otherwise `Fetcher.Discover` loads it once (feed timeout and size cap):
a feed is kept as given, an HTML page gives the first `<link rel="alternate">` of an RSS, Atom or JSON Feed
type, resolved against `<base href>` or the final address after redirects. Nothing found is
`ErrNoFeedFound`. Discovery runs outside the transaction, and its errors become `ValidationError`s on `url`.

## Implementation Flow
A feed is refreshed by one `feeds.refresh` job (`Refresh` in `pkg/service/feeds/refresh.go`). Every download
(the feed document, then the images of its new items) happens outside any transaction: `prepareNewItems`
finds the new items with read-only queries and `prepareItem` converts each to markdown and stores its images
on the plain store. Only then does one transaction lock the feed row, skip items another refresh saved
meanwhile, insert the rest with the subscribers' rows and save the feed's counters. Images stored for an item
that another refresh saved first stay stored.

1. **`prepareItem`** (`pkg/service/feeds/refresh.go`) - Creates global timeout context and upload function
2. **`CreateImageReplacer`** (`pkg/feedops/reader/cleaner.go`) - Extracts image URLs, enforces max images limit, handles errors
3. **`FetchMedia`** (`pkg/feedops/reader/fetcher.go`) - Downloads images with size/timeout limits, validates MIME types
4. **`StoreUpload`** (`pkg/service/media/media.go`) - Stores images linked to RSS feed ID
5. **`ReplaceImageUrlsOrLinkify`** (`pkg/markdown/modify.go`) - Replaces URLs in markdown AST

## Error Handling
Failed downloads are replaced with readable error messages in markdown:
- `_[Image download timed out: URL]_`
- `_[Image too large: URL]_`
- `_[Image limit exceeded (20 max): URL]_`
- `_[Image download failed: URL]_`

## Database
- `media_uploads` table supports either `user_id` OR `rss_feed_id` (mutual exclusivity enforced)
- Migration: `migrations/20251231005904-media_uploads_feeds.sql`
