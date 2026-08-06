DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS device_tokens;
DROP TABLE IF EXISTS notifications;
ALTER TABLE IF EXISTS chat_participants DROP CONSTRAINT IF EXISTS fk_chat_participants_last_read;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_participants;
DROP TABLE IF EXISTS chat_rooms;
