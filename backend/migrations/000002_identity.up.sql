-- 000002 · Identity, access control, sessions, files

-- ─── files ──────────────────────────────────────────────────────────
-- Declared before users because users.avatar_file_id points at it; the
-- reverse FK (files.owner_user_id) is added once users exists.
CREATE TABLE files (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id   UUID,
    bucket          VARCHAR(64)  NOT NULL,
    object_key      VARCHAR(512) NOT NULL,
    original_name   VARCHAR(255) NOT NULL,
    mime_type       VARCHAR(128) NOT NULL,
    size_bytes      BIGINT       NOT NULL CHECK (size_bytes >= 0),
    checksum_sha256 CHAR(64),
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private'
                    CHECK (visibility IN ('private', 'public')),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_files_object_key ON files (bucket, object_key) WHERE deleted_at IS NULL;
CREATE INDEX idx_files_owner ON files (owner_user_id) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_files_updated_at BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── users ──────────────────────────────────────────────────────────
CREATE TABLE users (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone                 VARCHAR(20)  NOT NULL,
    email                 VARCHAR(255),
    password_hash         VARCHAR(255) NOT NULL,
    first_name            VARCHAR(100) NOT NULL DEFAULT '',
    last_name             VARCHAR(100) NOT NULL DEFAULT '',
    middle_name           VARCHAR(100),
    avatar_file_id        UUID REFERENCES files (id) ON DELETE SET NULL,
    birth_date            DATE,
    gender                VARCHAR(10) CHECK (gender IN ('male', 'female')),
    locale                VARCHAR(5)   NOT NULL DEFAULT 'uz'
                          CHECK (locale IN ('uz', 'ru', 'en')),
    status                VARCHAR(24)  NOT NULL DEFAULT 'pending_verification'
                          CHECK (status IN ('pending_verification', 'active', 'blocked', 'deleted')),
    blocked_reason        TEXT,
    blocked_at            TIMESTAMPTZ,
    blocked_by            UUID,
    phone_verified_at     TIMESTAMPTZ,
    email_verified_at     TIMESTAMPTZ,
    last_login_at         TIMESTAMPTZ,
    last_login_ip         INET,
    failed_login_attempts SMALLINT     NOT NULL DEFAULT 0,
    locked_until          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ,
    CONSTRAINT chk_users_phone_format CHECK (phone ~ '^\+[1-9][0-9]{7,14}$')
);
CREATE UNIQUE INDEX idx_users_phone ON users (phone) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_email ON users (lower(email)) WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_status ON users (status) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users (created_at DESC);
CREATE INDEX idx_users_name_trgm ON users USING GIN ((first_name || ' ' || last_name) gin_trgm_ops);
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE users ADD CONSTRAINT fk_users_blocked_by
    FOREIGN KEY (blocked_by) REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE files ADD CONSTRAINT fk_files_owner
    FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE SET NULL;

-- ─── roles & permissions ────────────────────────────────────────────
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(32)  NOT NULL UNIQUE,
    name        VARCHAR(64)  NOT NULL,
    description TEXT,
    level       SMALLINT     NOT NULL DEFAULT 0,
    is_system   BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_roles_updated_at BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(64) NOT NULL UNIQUE,
    resource    VARCHAR(32) NOT NULL,
    action      VARCHAR(32) NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_permissions_resource ON permissions (resource);

CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);
CREATE INDEX idx_role_permissions_permission ON role_permissions (permission_id);

CREATE TABLE user_roles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id    UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    company_id UUID,
    granted_by UUID REFERENCES users (id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_user_roles_unique
    ON user_roles (user_id, role_id, COALESCE(company_id, '00000000-0000-0000-0000-000000000000'::UUID));
CREATE INDEX idx_user_roles_user ON user_roles (user_id);
CREATE INDEX idx_user_roles_company ON user_roles (company_id) WHERE company_id IS NOT NULL;
CREATE TRIGGER trg_user_roles_updated_at BEFORE UPDATE ON user_roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── sessions (refresh token rotation) ──────────────────────────────
CREATE TABLE sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash CHAR(64) NOT NULL,
    family_id          UUID NOT NULL,
    replaced_by        UUID REFERENCES sessions (id) ON DELETE SET NULL,
    device_id          VARCHAR(128),
    device_name        VARCHAR(128),
    platform           VARCHAR(16) NOT NULL DEFAULT 'web'
                       CHECK (platform IN ('android', 'ios', 'web', 'unknown')),
    app_version        VARCHAR(32),
    ip_address         INET,
    user_agent         TEXT,
    last_used_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    revoked_reason     VARCHAR(64),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_sessions_token_hash ON sessions (refresh_token_hash);
CREATE INDEX idx_sessions_user_active ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_sessions_family ON sessions (family_id);
CREATE INDEX idx_sessions_expires ON sessions (expires_at) WHERE revoked_at IS NULL;
CREATE TRIGGER trg_sessions_updated_at BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── one-time codes ─────────────────────────────────────────────────
CREATE TABLE otp_codes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone        VARCHAR(20) NOT NULL,
    code_hash    CHAR(64)    NOT NULL,
    purpose      VARCHAR(32) NOT NULL
                 CHECK (purpose IN ('register', 'login', 'reset_password', 'change_phone')),
    attempts     SMALLINT    NOT NULL DEFAULT 0,
    max_attempts SMALLINT    NOT NULL DEFAULT 5,
    ip_address   INET,
    expires_at   TIMESTAMPTZ NOT NULL,
    consumed_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_otp_lookup ON otp_codes (phone, purpose, created_at DESC);
CREATE INDEX idx_otp_expires ON otp_codes (expires_at) WHERE consumed_at IS NULL;
CREATE TRIGGER trg_otp_updated_at BEFORE UPDATE ON otp_codes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE password_resets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    ip_address INET,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_password_resets_user ON password_resets (user_id) WHERE used_at IS NULL;
