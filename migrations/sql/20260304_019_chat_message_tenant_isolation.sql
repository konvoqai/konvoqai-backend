DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chat_conversations_id_user_key'
  ) THEN
    ALTER TABLE chat_conversations
      ADD CONSTRAINT chat_conversations_id_user_key UNIQUE (id, user_id);
  END IF;
END $$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chat_messages_conversation_id_fkey'
  ) THEN
    ALTER TABLE chat_messages
      DROP CONSTRAINT chat_messages_conversation_id_fkey;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chat_messages_conversation_user_fkey'
  ) THEN
    ALTER TABLE chat_messages
      ADD CONSTRAINT chat_messages_conversation_user_fkey
      FOREIGN KEY (conversation_id, user_id)
      REFERENCES chat_conversations(id, user_id)
      ON DELETE CASCADE
      NOT VALID;
  END IF;
END $$;
