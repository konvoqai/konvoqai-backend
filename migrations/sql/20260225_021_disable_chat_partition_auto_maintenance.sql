-- Migration: 20260225_021_disable_chat_partition_auto_maintenance
--
-- Disables automatic monthly chat_messages partition creation.
-- Existing partitions are left untouched to avoid data loss.

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_extension WHERE extname = 'pg_cron'
  ) THEN
    PERFORM cron.unschedule('ensure-chat-partitions')
    WHERE EXISTS (
      SELECT 1 FROM cron.job WHERE jobname = 'ensure-chat-partitions'
    );
  END IF;
END $$;

DROP FUNCTION IF EXISTS ensure_chat_message_partitions();
