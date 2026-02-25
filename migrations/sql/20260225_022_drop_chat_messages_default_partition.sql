-- Migration: 20260225_022_drop_chat_messages_default_partition
--
-- Removes chat_messages_default partition safely:
-- 1) If it contains rows, create required monthly partitions.
-- 2) Move rows into the parent table (which routes into monthly partitions).
-- 3) Detach and drop the default partition.

DO $$
DECLARE
  parent_table      regclass := to_regclass('public.chat_messages');
  default_table     regclass := to_regclass('public.chat_messages_default');
  is_attached       BOOLEAN;
  row_count         BIGINT;
  min_created_at    TIMESTAMP;
  max_created_at    TIMESTAMP;
  month_start       DATE;
  month_end         DATE;
BEGIN
  IF parent_table IS NULL THEN
    RAISE NOTICE 'table public.chat_messages does not exist; skipping default partition cleanup.';
    RETURN;
  END IF;

  IF default_table IS NULL THEN
    RAISE NOTICE 'table public.chat_messages_default does not exist; nothing to do.';
    RETURN;
  END IF;

  SELECT EXISTS (
    SELECT 1
    FROM pg_inherits
    WHERE inhparent = parent_table
      AND inhrelid = default_table
  ) INTO is_attached;

  IF NOT is_attached THEN
    RAISE NOTICE 'public.chat_messages_default is not attached; dropping orphan table.';
    EXECUTE 'DROP TABLE IF EXISTS public.chat_messages_default';
    RETURN;
  END IF;

  EXECUTE 'SELECT count(*), min(created_at), max(created_at) FROM public.chat_messages_default'
    INTO row_count, min_created_at, max_created_at;

  IF row_count > 0 THEN
    month_start := date_trunc('month', min_created_at)::date;

    WHILE month_start <= date_trunc('month', max_created_at)::date LOOP
      month_end := (month_start + INTERVAL '1 month')::date;
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF public.chat_messages FOR VALUES FROM (%L) TO (%L)',
        'chat_messages_' || to_char(month_start, 'YYYYMM'),
        month_start::text,
        month_end::text
      );
      month_start := month_end;
    END LOOP;

    EXECUTE 'INSERT INTO public.chat_messages SELECT * FROM public.chat_messages_default';
    EXECUTE 'DELETE FROM public.chat_messages_default';
  END IF;

  EXECUTE 'ALTER TABLE public.chat_messages DETACH PARTITION public.chat_messages_default';
  EXECUTE 'DROP TABLE IF EXISTS public.chat_messages_default';
END $$;
