CREATE TABLE IF NOT EXISTS scraper_sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_url TEXT NOT NULL,
  source_title TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, source_url)
);

CREATE INDEX IF NOT EXISTS idx_scraper_sources_user_created
  ON scraper_sources(user_id, created_at DESC);
