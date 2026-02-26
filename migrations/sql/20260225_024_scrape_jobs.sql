CREATE TABLE IF NOT EXISTS scrape_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_url TEXT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'queued',
  progress INTEGER NOT NULL DEFAULT 0,
  message TEXT,
  error_message TEXT,
  started_at TIMESTAMP,
  completed_at TIMESTAMP,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scrape_jobs_user_created_at
  ON scrape_jobs(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_scrape_jobs_user_status
  ON scrape_jobs(user_id, status);

DROP TRIGGER IF EXISTS update_scrape_jobs_updated_at ON scrape_jobs;
CREATE TRIGGER update_scrape_jobs_updated_at
BEFORE UPDATE ON scrape_jobs
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();
