DROP TABLE IF EXISTS user_external_identities;
DROP TABLE IF EXISTS system_settings;
ALTER TABLE outbox_messages DROP COLUMN IF EXISTS provider;
ALTER TABLE operations DROP COLUMN IF EXISTS message_provider;
ALTER TABLE operations DROP COLUMN IF EXISTS message_conversation_id;
ALTER TABLE operations DROP COLUMN IF EXISTS message_event_id;
UPDATE users SET feishu_open_id = 'legacy-local:' || id::text WHERE feishu_open_id IS NULL;
ALTER TABLE users ALTER COLUMN feishu_open_id SET NOT NULL;
