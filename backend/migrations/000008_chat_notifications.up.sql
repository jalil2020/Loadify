-- 000008 · Chat rooms, messages, notifications, device tokens

CREATE TABLE chat_rooms (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id      UUID REFERENCES bookings (id) ON DELETE CASCADE,
    load_id         UUID REFERENCES loads (id) ON DELETE SET NULL,
    trip_id         UUID REFERENCES trips (id) ON DELETE SET NULL,
    type            VARCHAR(16) NOT NULL DEFAULT 'booking'
                    CHECK (type IN ('booking', 'support', 'direct')),
    status          VARCHAR(16) NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'archived', 'closed')),
    last_message_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_chat_rooms_booking ON chat_rooms (booking_id) WHERE booking_id IS NOT NULL;
CREATE INDEX idx_chat_rooms_activity ON chat_rooms (last_message_at DESC NULLS LAST);
CREATE TRIGGER trg_chat_rooms_updated_at BEFORE UPDATE ON chat_rooms
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE chat_participants (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id              UUID NOT NULL REFERENCES chat_rooms (id) ON DELETE CASCADE,
    user_id              UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role                 VARCHAR(16) NOT NULL DEFAULT 'member',
    last_read_message_id UUID,
    unread_count         INTEGER NOT NULL DEFAULT 0,
    is_muted             BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at              TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (room_id, user_id)
);
CREATE INDEX idx_chat_participants_user ON chat_participants (user_id) WHERE left_at IS NULL;
CREATE TRIGGER trg_chat_participants_updated_at BEFORE UPDATE ON chat_participants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE chat_messages (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id        UUID NOT NULL REFERENCES chat_rooms (id) ON DELETE CASCADE,
    sender_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    type           VARCHAR(16) NOT NULL DEFAULT 'text'
                   CHECK (type IN ('text', 'image', 'file', 'location', 'system')),
    body           TEXT,
    file_id        UUID REFERENCES files (id) ON DELETE SET NULL,
    location       geography(Point, 4326),
    reply_to_id    UUID REFERENCES chat_messages (id) ON DELETE SET NULL,
    client_msg_id  VARCHAR(64),
    delivered_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at      TIMESTAMPTZ,
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT chk_chat_messages_content CHECK (
        (type = 'text'     AND body IS NOT NULL) OR
        (type IN ('image', 'file') AND file_id IS NOT NULL) OR
        (type = 'location' AND location IS NOT NULL) OR
        (type = 'system')
    )
);
CREATE INDEX idx_chat_messages_room ON chat_messages (room_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_chat_messages_client_id ON chat_messages (room_id, sender_user_id, client_msg_id)
    WHERE client_msg_id IS NOT NULL;
CREATE TRIGGER trg_chat_messages_updated_at BEFORE UPDATE ON chat_messages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE chat_participants ADD CONSTRAINT fk_chat_participants_last_read
    FOREIGN KEY (last_read_message_id) REFERENCES chat_messages (id) ON DELETE SET NULL;

CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type       VARCHAR(48) NOT NULL,
    title      VARCHAR(255) NOT NULL,
    body       TEXT NOT NULL,
    data       JSONB NOT NULL DEFAULT '{}'::JSONB,
    channel    VARCHAR(16) NOT NULL DEFAULT 'in_app'
               CHECK (channel IN ('push', 'in_app', 'sms', 'email')),
    priority   VARCHAR(8) NOT NULL DEFAULT 'normal'
               CHECK (priority IN ('low', 'normal', 'high')),
    image_url  VARCHAR(512),
    action_url VARCHAR(512),
    read_at    TIMESTAMPTZ,
    sent_at    TIMESTAMPTZ,
    failed_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_inbox ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE read_at IS NULL;
CREATE INDEX idx_notifications_type ON notifications (type, created_at DESC);

CREATE TABLE device_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token        VARCHAR(512) NOT NULL,
    platform     VARCHAR(16) NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
    device_id    VARCHAR(128),
    device_model VARCHAR(128),
    app_version  VARCHAR(32),
    locale       VARCHAR(5),
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_device_tokens_token ON device_tokens (token);
CREATE INDEX idx_device_tokens_user ON device_tokens (user_id) WHERE is_active;
CREATE TRIGGER trg_device_tokens_updated_at BEFORE UPDATE ON device_tokens
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_preferences (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type       VARCHAR(48) NOT NULL,
    push       BOOLEAN NOT NULL DEFAULT TRUE,
    in_app     BOOLEAN NOT NULL DEFAULT TRUE,
    sms        BOOLEAN NOT NULL DEFAULT FALSE,
    email      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, type)
);
CREATE TRIGGER trg_notification_preferences_updated_at BEFORE UPDATE ON notification_preferences
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
