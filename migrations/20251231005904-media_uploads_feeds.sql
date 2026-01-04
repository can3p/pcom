
-- +migrate Up

-- Make user_id nullable and add rss_feed_id column
ALTER TABLE media_uploads
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN rss_feed_id UUID REFERENCES rss_feeds(id);

-- Add constraint to ensure either user_id or rss_feed_id is set (but not both)
ALTER TABLE media_uploads
    ADD CONSTRAINT media_uploads_owner_check 
    CHECK (
        (user_id IS NOT NULL AND rss_feed_id IS NULL) OR
        (user_id IS NULL AND rss_feed_id IS NOT NULL)
    );

-- Add index for rss_feed_id lookups
CREATE INDEX idx_media_uploads_rss_feed_id ON media_uploads(rss_feed_id);

-- +migrate Down

DROP INDEX IF EXISTS idx_media_uploads_rss_feed_id;
ALTER TABLE media_uploads DROP CONSTRAINT IF EXISTS media_uploads_owner_check;
ALTER TABLE media_uploads DROP COLUMN IF EXISTS rss_feed_id;
ALTER TABLE media_uploads ALTER COLUMN user_id SET NOT NULL;
