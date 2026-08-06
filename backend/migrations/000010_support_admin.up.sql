-- 000010 · Complaints, support, audit logs, settings, CMS

CREATE TABLE complaints (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_type      VARCHAR(16) NOT NULL
                     CHECK (target_type IN ('user', 'driver', 'company', 'load', 'trip')),
    target_id        UUID NOT NULL,
    trip_id          UUID REFERENCES trips (id) ON DELETE SET NULL,
    category         VARCHAR(24) NOT NULL
                     CHECK (category IN ('fraud', 'no_show', 'damage', 'payment',
                                         'behaviour', 'documents', 'other')),
    description      TEXT NOT NULL,
    evidence_file_ids UUID[] NOT NULL DEFAULT '{}',
    status           VARCHAR(16) NOT NULL DEFAULT 'open'
                     CHECK (status IN ('open', 'in_review', 'resolved', 'rejected')),
    priority         VARCHAR(8) NOT NULL DEFAULT 'medium'
                     CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    assigned_to      UUID REFERENCES users (id) ON DELETE SET NULL,
    resolution       TEXT,
    resolved_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_complaints_status ON complaints (status, priority, created_at);
CREATE INDEX idx_complaints_target ON complaints (target_type, target_id);
CREATE INDEX idx_complaints_reporter ON complaints (reporter_user_id, created_at DESC);
CREATE INDEX idx_complaints_assignee ON complaints (assigned_to) WHERE status IN ('open', 'in_review');
CREATE TRIGGER trg_complaints_updated_at BEFORE UPDATE ON complaints
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE support_tickets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    subject     VARCHAR(255) NOT NULL,
    category    VARCHAR(32) NOT NULL DEFAULT 'general',
    status      VARCHAR(16) NOT NULL DEFAULT 'open'
                CHECK (status IN ('open', 'pending', 'answered', 'closed')),
    priority    VARCHAR(8) NOT NULL DEFAULT 'medium'
                CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    assigned_to UUID REFERENCES users (id) ON DELETE SET NULL,
    closed_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_support_tickets_user ON support_tickets (user_id, created_at DESC);
CREATE INDEX idx_support_tickets_queue ON support_tickets (status, priority, created_at);
CREATE TRIGGER trg_support_tickets_updated_at BEFORE UPDATE ON support_tickets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE support_messages (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id      UUID NOT NULL REFERENCES support_tickets (id) ON DELETE CASCADE,
    sender_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    is_staff       BOOLEAN NOT NULL DEFAULT FALSE,
    body           TEXT NOT NULL,
    file_ids       UUID[] NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_support_messages_ticket ON support_messages (ticket_id, created_at);

CREATE TABLE audit_logs (
    id            BIGSERIAL PRIMARY KEY,
    actor_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    actor_role    VARCHAR(32),
    action        VARCHAR(64) NOT NULL,
    resource_type VARCHAR(48) NOT NULL,
    resource_id   UUID,
    old_values    JSONB,
    new_values    JSONB,
    description   TEXT,
    ip_address    INET,
    user_agent    TEXT,
    request_id    VARCHAR(64),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_resource ON audit_logs (resource_type, resource_id, created_at DESC);
CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_user_id, created_at DESC);
CREATE INDEX idx_audit_logs_action ON audit_logs (action, created_at DESC);
CREATE INDEX idx_audit_logs_created ON audit_logs (created_at DESC);

CREATE TABLE system_settings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key         VARCHAR(64) NOT NULL UNIQUE,
    value       JSONB NOT NULL,
    value_type  VARCHAR(16) NOT NULL DEFAULT 'string'
                CHECK (value_type IN ('string', 'number', 'boolean', 'json')),
    group_name  VARCHAR(32) NOT NULL DEFAULT 'general',
    description TEXT,
    is_public   BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_system_settings_group ON system_settings (group_name);
CREATE TRIGGER trg_system_settings_updated_at BEFORE UPDATE ON system_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE feature_flags (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key             VARCHAR(64) NOT NULL UNIQUE,
    description     TEXT,
    is_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    rollout_percent SMALLINT NOT NULL DEFAULT 0 CHECK (rollout_percent BETWEEN 0 AND 100),
    conditions      JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_feature_flags_updated_at BEFORE UPDATE ON feature_flags
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cms_pages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         VARCHAR(100) NOT NULL UNIQUE,
    title_uz     VARCHAR(255) NOT NULL,
    title_ru     VARCHAR(255) NOT NULL,
    title_en     VARCHAR(255) NOT NULL,
    body_uz      TEXT NOT NULL DEFAULT '',
    body_ru      TEXT NOT NULL DEFAULT '',
    body_en      TEXT NOT NULL DEFAULT '',
    meta_title   VARCHAR(255),
    meta_desc    VARCHAR(512),
    status       VARCHAR(16) NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft', 'published')),
    published_at TIMESTAMPTZ,
    updated_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_cms_pages_updated_at BEFORE UPDATE ON cms_pages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE faqs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category    VARCHAR(32) NOT NULL DEFAULT 'general',
    question_uz VARCHAR(512) NOT NULL,
    question_ru VARCHAR(512) NOT NULL,
    question_en VARCHAR(512) NOT NULL,
    answer_uz   TEXT NOT NULL,
    answer_ru   TEXT NOT NULL,
    answer_en   TEXT NOT NULL,
    audience    VARCHAR(16) NOT NULL DEFAULT 'all'
                CHECK (audience IN ('all', 'driver', 'company')),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order  SMALLINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_faqs_listing ON faqs (audience, category, sort_order) WHERE is_active;
CREATE TRIGGER trg_faqs_updated_at BEFORE UPDATE ON faqs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE banners (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       VARCHAR(255) NOT NULL,
    image_file_id UUID REFERENCES files (id) ON DELETE SET NULL,
    action_url  VARCHAR(512),
    audience    VARCHAR(16) NOT NULL DEFAULT 'all'
                CHECK (audience IN ('all', 'driver', 'company')),
    placement   VARCHAR(32) NOT NULL DEFAULT 'home',
    starts_at   TIMESTAMPTZ,
    ends_at     TIMESTAMPTZ,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order  SMALLINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_banners_active ON banners (audience, placement, sort_order)
    WHERE is_active;
CREATE TRIGGER trg_banners_updated_at BEFORE UPDATE ON banners
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
