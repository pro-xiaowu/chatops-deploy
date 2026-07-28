ALTER TABLE users ALTER COLUMN feishu_open_id DROP NOT NULL;
ALTER TABLE operations ADD COLUMN message_provider text NOT NULL DEFAULT 'web';
ALTER TABLE operations ADD COLUMN message_conversation_id text NOT NULL DEFAULT '';
ALTER TABLE operations ADD COLUMN message_event_id text NOT NULL DEFAULT '';
ALTER TABLE outbox_messages ADD COLUMN provider text NOT NULL DEFAULT 'web';
CREATE TABLE system_settings (key text PRIMARY KEY, value text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE user_external_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK(provider IN ('web','feishu','wecom','dingtalk')),
    subject_id text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(provider, subject_id),
    UNIQUE(user_id, provider)
);
CREATE INDEX user_external_identities_user_idx ON user_external_identities(user_id);
