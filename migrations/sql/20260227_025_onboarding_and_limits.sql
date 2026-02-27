ALTER TABLE users
ADD COLUMN IF NOT EXISTS onboarding_completed_at TIMESTAMP;

ALTER TABLE scraper_sources
ADD COLUMN IF NOT EXISTS scraped_pages INTEGER NOT NULL DEFAULT 1;

UPDATE scraper_sources
SET scraped_pages = 1
WHERE scraped_pages IS NULL OR scraped_pages <= 0;

