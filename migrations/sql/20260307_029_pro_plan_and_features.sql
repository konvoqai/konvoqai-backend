-- Migration: Add PRO plan tier + feature tables (persona, navigation, branding, CRM, follow-up, hybrid inbox, flows)

-- 1. Add 'pro' to plan_type constraint
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_plan_type_check;
ALTER TABLE users ADD CONSTRAINT users_plan_type_check
  CHECK (plan_type IN ('free', 'basic', 'pro', 'enterprise'));

ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_plan_type_check;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_plan_type_check
  CHECK (plan_type IN ('free', 'basic', 'pro', 'enterprise'));

-- 2. Update conversation limits to match new tier targets
UPDATE users SET conversations_limit = 300  WHERE plan_type = 'free'  AND (conversations_limit = 100 OR conversations_limit IS NULL);
UPDATE users SET conversations_limit = 1500 WHERE plan_type = 'basic' AND (conversations_limit = 1000 OR conversations_limit IS NULL);

-- 3. CRM fields on leads table
ALTER TABLE leads ADD COLUMN IF NOT EXISTS pipeline_stage VARCHAR(20) NOT NULL DEFAULT 'new';
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_pipeline_stage_check;
ALTER TABLE leads ADD CONSTRAINT leads_pipeline_stage_check
  CHECK (pipeline_stage IN ('new', 'contacted', 'qualified', 'proposal', 'won', 'lost'));
ALTER TABLE leads ADD COLUMN IF NOT EXISTS crm_notes TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS next_follow_up_at TIMESTAMPTZ;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS tags TEXT[];

-- 4. Auto follow-up config table
CREATE TABLE IF NOT EXISTS follow_up_configs (
  id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  is_active         BOOLEAN      NOT NULL DEFAULT FALSE,
  delay_hours       INT          NOT NULL DEFAULT 24,
  trigger_event     VARCHAR(50)  NOT NULL DEFAULT 'lead_created',
  template_subject  TEXT         NOT NULL DEFAULT '',
  template_body     TEXT         NOT NULL DEFAULT '',
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id)
);

-- 5. Hybrid AI+Human handoff tables
CREATE TABLE IF NOT EXISTS handoff_requests (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  session_id      VARCHAR(255) NOT NULL,
  status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
  claimed_by      VARCHAR(255),
  visitor_name    VARCHAR(255),
  visitor_email   VARCHAR(255),
  trigger_reason  TEXT,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT handoff_requests_status_check CHECK (status IN ('pending', 'claimed', 'resolved'))
);
CREATE INDEX IF NOT EXISTS idx_handoff_user_status ON handoff_requests(user_id, status);
CREATE INDEX IF NOT EXISTS idx_handoff_session ON handoff_requests(session_id);

CREATE TABLE IF NOT EXISTS handoff_messages (
  id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  handoff_id   UUID         NOT NULL REFERENCES handoff_requests(id) ON DELETE CASCADE,
  sender_type  VARCHAR(20)  NOT NULL,
  sender_email VARCHAR(255),
  content      TEXT         NOT NULL,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT handoff_messages_sender_type_check CHECK (sender_type IN ('visitor', 'agent'))
);
CREATE INDEX IF NOT EXISTS idx_handoff_messages_handoff ON handoff_messages(handoff_id, created_at);

-- 6. Conversation flows table
CREATE TABLE IF NOT EXISTS conversation_flows (
  id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       VARCHAR(255) NOT NULL,
  flow_data  JSONB        NOT NULL DEFAULT '{"nodes": [], "edges": []}',
  is_active  BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_flows_user ON conversation_flows(user_id);
