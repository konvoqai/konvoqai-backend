-- Migration: 20260221_013_partition_maintenance
--
-- Creates a reusable helper function for creating the next 3 calendar months of
-- chat_messages partitions.
--
-- NOTE: this migration only defines the helper; it does not execute it and does
-- not schedule it automatically.

-- --- Partition helper function -----------------------------------------------

CREATE OR REPLACE FUNCTION ensure_chat_message_partitions()
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  i            INTEGER;
  target_start DATE;
  target_end   DATE;
  table_name   TEXT;
BEGIN
  -- Always keep the current month plus the next 2 months created
  FOR i IN 0..2 LOOP
    target_start := date_trunc('month', CURRENT_DATE + (i || ' month')::INTERVAL)::date;
    target_end   := date_trunc('month', CURRENT_DATE + ((i + 1) || ' month')::INTERVAL)::date;
    table_name   := 'chat_messages_' || to_char(target_start, 'YYYYMM');

    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS %I PARTITION OF chat_messages FOR VALUES FROM (%L) TO (%L)',
      table_name,
      target_start::text,
      target_end::text
    );
  END LOOP;
END;
$$;

-- To execute manually (for example from maintenance tooling), run:
-- SELECT ensure_chat_message_partitions();

-- --- widget_analytics cleanup function ---------------------------------------

CREATE OR REPLACE FUNCTION cleanup_old_widget_analytics(retention_days INTEGER DEFAULT 90)
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
  deleted_count INTEGER;
BEGIN
  DELETE FROM widget_analytics
  WHERE created_at < NOW() - (retention_days || ' days')::INTERVAL;

  GET DIAGNOSTICS deleted_count = ROW_COUNT;
  RETURN deleted_count;
END;
$$;

-- Optionally schedule analytics cleanup via pg_cron as well
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_extension WHERE extname = 'pg_cron'
  ) THEN
    PERFORM cron.unschedule('cleanup-widget-analytics')
    WHERE EXISTS (
      SELECT 1 FROM cron.job WHERE jobname = 'cleanup-widget-analytics'
    );

    -- Run every Sunday at 02:00 UTC
    PERFORM cron.schedule(
      'cleanup-widget-analytics',
      '0 2 * * 0',
      'SELECT cleanup_old_widget_analytics(90)'
    );

    RAISE NOTICE 'pg_cron schedule "cleanup-widget-analytics" created.';
  END IF;
END $$;
