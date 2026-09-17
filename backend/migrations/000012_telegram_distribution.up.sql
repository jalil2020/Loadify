-- 000012 · Telegram distribution: accounts, groups, load ads, posting jobs
--
-- Every table carries company_id and is reachable only through a composite
-- foreign key (id, company_id). A row from one company therefore cannot
-- reference a row from another even if the application layer gets it wrong.

-- ─── telegram accounts ──────────────────────────────────────────────
-- The session belongs to a person, not to a company: it is that person's own
-- Telegram account. user_id is who consented, and only they may post with it.
CREATE TABLE telegram_accounts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tg_user_id          BIGINT,
    phone               VARCHAR(20) NOT NULL,
    label               VARCHAR(64) NOT NULL DEFAULT '',
    session_ciphertext  BYTEA,
    session_key_version SMALLINT,
    status              VARCHAR(16) NOT NULL DEFAULT 'connecting'
                        CHECK (status IN ('connecting', 'active', 'paused',
                                          'restricted', 'revoked', 'disabled')),
    status_reason       TEXT,
    paused_until        TIMESTAMPTZ,
    consent_version     VARCHAR(16),
    consent_at          TIMESTAMPTZ,
    last_used_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT uq_telegram_accounts_tenant UNIQUE (id, company_id),
    CONSTRAINT chk_telegram_accounts_phone CHECK (phone ~ '^\+[1-9][0-9]{7,14}$'),
    -- A sealed session without its key version is unopenable, and a version
    -- without a session is a leftover; neither state is ever correct.
    CONSTRAINT chk_telegram_accounts_session
        CHECK ((session_ciphertext IS NULL) = (session_key_version IS NULL)),
    -- Posting requires a session and recorded consent. Telegram's terms forbid
    -- acting for someone without their knowledge, so the database says so too.
    CONSTRAINT chk_telegram_accounts_active
        CHECK (status <> 'active' OR (session_ciphertext IS NOT NULL AND consent_at IS NOT NULL))
);
CREATE UNIQUE INDEX idx_telegram_accounts_phone
    ON telegram_accounts (user_id, phone) WHERE deleted_at IS NULL;
-- One Telegram account belongs to one platform account: connecting the same
-- account twice is how a limit gets multiplied.
CREATE UNIQUE INDEX idx_telegram_accounts_tg_user
    ON telegram_accounts (tg_user_id) WHERE tg_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_telegram_accounts_company
    ON telegram_accounts (company_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_telegram_accounts_paused
    ON telegram_accounts (paused_until) WHERE status = 'paused';
CREATE TRIGGER trg_telegram_accounts_updated_at BEFORE UPDATE ON telegram_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── groups the account is already a member of ──────────────────────
CREATE TABLE telegram_groups (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL,
    account_id       UUID NOT NULL,
    tg_peer_id       BIGINT NOT NULL,
    access_hash      BIGINT,
    title            VARCHAR(255) NOT NULL DEFAULT '',
    username         VARCHAR(64),
    kind             VARCHAR(16) NOT NULL DEFAULT 'supergroup'
                     CHECK (kind IN ('group', 'supergroup', 'channel')),
    language         VARCHAR(5) NOT NULL DEFAULT 'ru'
                     CHECK (language IN ('uz', 'ru', 'en')),
    member_count     INTEGER,
    slowmode_seconds INTEGER NOT NULL DEFAULT 0 CHECK (slowmode_seconds >= 0),
    can_send         BOOLEAN NOT NULL DEFAULT TRUE,
    posting_enabled  BOOLEAN NOT NULL DEFAULT TRUE,
    active           BOOLEAN NOT NULL DEFAULT TRUE,
    inactive_reason  VARCHAR(64),
    last_synced_at   TIMESTAMPTZ,
    last_posted_at   TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT uq_telegram_groups_tenant UNIQUE (id, company_id),
    CONSTRAINT fk_telegram_groups_account
        FOREIGN KEY (account_id, company_id)
        REFERENCES telegram_accounts (id, company_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_telegram_groups_peer
    ON telegram_groups (account_id, tg_peer_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_telegram_groups_postable
    ON telegram_groups (company_id) WHERE deleted_at IS NULL AND active AND posting_enabled;
CREATE TRIGGER trg_telegram_groups_updated_at BEFORE UPDATE ON telegram_groups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- What the group accepts, as the logistician describes it. These are the only
-- group facts the matcher reads: nothing fetched from Telegram is fed to a model.
CREATE TABLE telegram_group_rules (
    group_id         UUID PRIMARY KEY,
    company_id       UUID NOT NULL,
    cargo_type_codes TEXT[] NOT NULL DEFAULT '{}',
    vehicle_types    TEXT[] NOT NULL DEFAULT '{}',
    min_weight_kg    INTEGER CHECK (min_weight_kg IS NULL OR min_weight_kg > 0),
    max_weight_kg    INTEGER CHECK (max_weight_kg IS NULL OR max_weight_kg > 0),
    hourly_limit     SMALLINT NOT NULL DEFAULT 2 CHECK (hourly_limit > 0),
    daily_limit      SMALLINT NOT NULL DEFAULT 3 CHECK (daily_limit > 0),
    cooldown_minutes INTEGER NOT NULL DEFAULT 60 CHECK (cooldown_minutes >= 0),
    quiet_from_hour  SMALLINT CHECK (quiet_from_hour BETWEEN 0 AND 23),
    quiet_to_hour    SMALLINT CHECK (quiet_to_hour BETWEEN 0 AND 23),
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_telegram_group_rules_weight
        CHECK (min_weight_kg IS NULL OR max_weight_kg IS NULL OR min_weight_kg <= max_weight_kg),
    CONSTRAINT fk_telegram_group_rules_group
        FOREIGN KEY (group_id, company_id)
        REFERENCES telegram_groups (id, company_id) ON DELETE CASCADE
);
CREATE TRIGGER trg_telegram_group_rules_updated_at BEFORE UPDATE ON telegram_group_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A group may serve several corridors, so routes are rows rather than columns.
-- A NULL region means the whole country.
CREATE TABLE telegram_group_routes (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id         UUID NOT NULL,
    company_id       UUID NOT NULL,
    origin_country   CHAR(2),
    origin_region_id UUID REFERENCES regions (id) ON DELETE CASCADE,
    dest_country     CHAR(2),
    dest_region_id   UUID REFERENCES regions (id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_telegram_group_routes_origin
        CHECK (origin_country IS NOT NULL OR origin_region_id IS NOT NULL),
    CONSTRAINT chk_telegram_group_routes_dest
        CHECK (dest_country IS NOT NULL OR dest_region_id IS NOT NULL),
    CONSTRAINT fk_telegram_group_routes_group
        FOREIGN KEY (group_id, company_id)
        REFERENCES telegram_groups (id, company_id) ON DELETE CASCADE
);
CREATE INDEX idx_telegram_group_routes_match
    ON telegram_group_routes (company_id, origin_country, dest_country);
CREATE INDEX idx_telegram_group_routes_group ON telegram_group_routes (group_id);

-- ─── load advertisements ────────────────────────────────────────────
-- Unlike loads, almost everything here is nullable: a Telegram advert routinely
-- omits the volume, the cargo type or the price, and inventing those values
-- would be the parser guessing. What was not said stays NULL, and the *_text
-- columns keep what the person actually wrote next to what we resolved.
CREATE TABLE load_ads (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    created_by_user_id  UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    raw_text            TEXT NOT NULL DEFAULT '',

    origin_city_id      UUID REFERENCES cities (id) ON DELETE SET NULL,
    origin_region_id    UUID REFERENCES regions (id) ON DELETE SET NULL,
    origin_country      CHAR(2),
    origin_text         VARCHAR(120) NOT NULL DEFAULT '',
    dest_city_id        UUID REFERENCES cities (id) ON DELETE SET NULL,
    dest_region_id      UUID REFERENCES regions (id) ON DELETE SET NULL,
    dest_country        CHAR(2),
    dest_text           VARCHAR(120) NOT NULL DEFAULT '',

    cargo_type_id       UUID REFERENCES cargo_types (id) ON DELETE SET NULL,
    cargo_text          VARCHAR(120),
    weight_kg           INTEGER CHECK (weight_kg IS NULL OR weight_kg > 0),
    volume_m3           NUMERIC(8, 2) CHECK (volume_m3 IS NULL OR volume_m3 > 0),
    vehicle_type        VARCHAR(24),
    loading_date        DATE,
    loading_date_text   VARCHAR(64),

    price_amount        BIGINT CHECK (price_amount IS NULL OR price_amount >= 0),
    price_currency      CHAR(3),
    price_negotiable    BOOLEAN NOT NULL DEFAULT FALSE,
    payment_terms       VARCHAR(16)
                        CHECK (payment_terms IS NULL OR payment_terms IN
                               ('on_loading', 'on_delivery', 'prepayment', 'deferred')),
    payment_method      VARCHAR(16)
                        CHECK (payment_method IS NULL OR payment_method IN
                               ('cash', 'transfer', 'card', 'escrow')),

    contact_phone       VARCHAR(20),
    contact_name        VARCHAR(120),
    notes               TEXT,

    -- Which rule or model produced each field, and the words it came from.
    field_sources       JSONB NOT NULL DEFAULT '{}'::JSONB,
    -- Normalised digest of the meaning, not the wording: editing the text does
    -- not slip the same advert past the duplicate check.
    fingerprint         CHAR(64) NOT NULL,
    status              VARCHAR(16) NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft', 'ready', 'publishing', 'published', 'archived')),
    marketplace_load_id UUID REFERENCES loads (id) ON DELETE SET NULL,
    published_at        TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT uq_load_ads_tenant UNIQUE (id, company_id),
    CONSTRAINT chk_load_ads_price
        CHECK (price_amount IS NULL OR price_currency IS NOT NULL)
);
CREATE INDEX idx_load_ads_company
    ON load_ads (company_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_load_ads_fingerprint
    ON load_ads (company_id, fingerprint) WHERE deleted_at IS NULL;
CREATE INDEX idx_load_ads_creator ON load_ads (created_by_user_id, created_at DESC);
-- The monthly advert allowance counts archived and deleted adverts too.
CREATE INDEX idx_load_ads_month ON load_ads (company_id, created_at);
CREATE TRIGGER trg_load_ads_updated_at BEFORE UPDATE ON load_ads
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── publishing ─────────────────────────────────────────────────────
-- One row per press of Publish: what was offered, what the guard approved and
-- what it refused, so a later question about a run has an answer.
CREATE TABLE publish_runs (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id           UUID NOT NULL,
    load_ad_id           UUID NOT NULL,
    account_id           UUID NOT NULL,
    requested_by_user_id UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    idempotency_key      VARCHAR(128) NOT NULL,
    candidates           INTEGER NOT NULL DEFAULT 0,
    approved             INTEGER NOT NULL DEFAULT 0,
    skipped              INTEGER NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_publish_runs_tenant UNIQUE (id, company_id),
    CONSTRAINT fk_publish_runs_load
        FOREIGN KEY (load_ad_id, company_id) REFERENCES load_ads (id, company_id) ON DELETE CASCADE,
    CONSTRAINT fk_publish_runs_account
        FOREIGN KEY (account_id, company_id)
        REFERENCES telegram_accounts (id, company_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_publish_runs_idempotency
    ON publish_runs (company_id, idempotency_key);
CREATE INDEX idx_publish_runs_load ON publish_runs (load_ad_id, created_at DESC);

CREATE TABLE posting_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL,
    run_id          UUID NOT NULL,
    load_ad_id      UUID NOT NULL,
    account_id      UUID NOT NULL,
    group_id        UUID NOT NULL,
    fingerprint     CHAR(64) NOT NULL,
    message_text    TEXT NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'processing', 'sent', 'failed',
                                      'skipped', 'rate_limited', 'cancelled')),
    skip_reason     VARCHAR(32),
    -- Generated once, before the first attempt. Telegram uses it to refuse a
    -- resend, which is what makes a retry after a crash safe.
    random_id       BIGINT NOT NULL,
    attempt         SMALLINT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    enqueued_at     TIMESTAMPTZ,
    lease_owner     VARCHAR(64),
    lease_until     TIMESTAMPTZ,
    tg_message_id   BIGINT,
    error_code      VARCHAR(48),
    error_detail    TEXT,
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_posting_jobs_tenant UNIQUE (id, company_id),
    CONSTRAINT fk_posting_jobs_run
        FOREIGN KEY (run_id, company_id) REFERENCES publish_runs (id, company_id) ON DELETE CASCADE,
    CONSTRAINT fk_posting_jobs_load
        FOREIGN KEY (load_ad_id, company_id) REFERENCES load_ads (id, company_id) ON DELETE CASCADE,
    CONSTRAINT fk_posting_jobs_account
        FOREIGN KEY (account_id, company_id)
        REFERENCES telegram_accounts (id, company_id) ON DELETE CASCADE,
    CONSTRAINT fk_posting_jobs_group
        FOREIGN KEY (group_id, company_id)
        REFERENCES telegram_groups (id, company_id) ON DELETE CASCADE
);
-- One advert may sit in a group's queue only once at a time.
CREATE UNIQUE INDEX idx_posting_jobs_active
    ON posting_jobs (load_ad_id, group_id)
    WHERE status IN ('pending', 'processing', 'rate_limited');
CREATE UNIQUE INDEX idx_posting_jobs_random ON posting_jobs (account_id, random_id);
CREATE INDEX idx_posting_jobs_due
    ON posting_jobs (next_attempt_at) WHERE status IN ('pending', 'rate_limited');
CREATE INDEX idx_posting_jobs_lease
    ON posting_jobs (lease_until) WHERE status = 'processing';
CREATE INDEX idx_posting_jobs_feed ON posting_jobs (company_id, created_at DESC);
CREATE INDEX idx_posting_jobs_group_sent
    ON posting_jobs (group_id, sent_at DESC) WHERE status = 'sent';
CREATE TRIGGER trg_posting_jobs_updated_at BEFORE UPDATE ON posting_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Append-only: the answer to "what happened to this post" must not be editable.
CREATE TABLE posting_history (
    id                BIGSERIAL PRIMARY KEY,
    job_id            UUID NOT NULL REFERENCES posting_jobs (id) ON DELETE CASCADE,
    company_id        UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    from_status       VARCHAR(16),
    to_status         VARCHAR(16) NOT NULL,
    attempt           SMALLINT NOT NULL DEFAULT 0,
    error_code        VARCHAR(48),
    detail            TEXT,
    -- Stored without access_hash, phone numbers or session fields.
    telegram_response JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_posting_history_job ON posting_history (job_id, created_at);
CREATE INDEX idx_posting_history_company ON posting_history (company_id, created_at DESC);

-- Why a post did not go out, kept separately so the anti-spam picture can be
-- read without scanning every job.
CREATE TABLE spam_events (
    id           BIGSERIAL PRIMARY KEY,
    company_id   UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    account_id   UUID REFERENCES telegram_accounts (id) ON DELETE SET NULL,
    group_id     UUID REFERENCES telegram_groups (id) ON DELETE SET NULL,
    job_id       UUID REFERENCES posting_jobs (id) ON DELETE SET NULL,
    kind         VARCHAR(24) NOT NULL
                 CHECK (kind IN ('duplicate', 'cooldown', 'group_limit', 'user_limit',
                                 'account_limit', 'plan_limit', 'quiet_hours', 'stale',
                                 'flood_wait', 'slowmode', 'peer_flood', 'write_forbidden')),
    wait_seconds INTEGER,
    detail       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_spam_events_company ON spam_events (company_id, created_at DESC);
CREATE INDEX idx_spam_events_account ON spam_events (account_id, created_at DESC);
CREATE INDEX idx_spam_events_kind ON spam_events (kind, created_at DESC);

-- ─── AI accounting ─────────────────────────────────────────────────
-- Tariff usage is counted from the records themselves (adverts, jobs, this
-- table), so there is no separate tally to keep in step.
CREATE TABLE ai_tasks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    user_id       UUID REFERENCES users (id) ON DELETE SET NULL,
    kind          VARCHAR(16) NOT NULL CHECK (kind IN ('parse', 'rank', 'assist', 'notify')),
    provider      VARCHAR(16) NOT NULL,
    model         VARCHAR(64) NOT NULL DEFAULT '',
    input_hash    CHAR(64) NOT NULL,
    cache_hit     BOOLEAN NOT NULL DEFAULT FALSE,
    status        VARCHAR(16) NOT NULL DEFAULT 'ok'
                  CHECK (status IN ('ok', 'rejected', 'failed')),
    reject_reason VARCHAR(48),
    tokens_in     INTEGER NOT NULL DEFAULT 0,
    tokens_out    INTEGER NOT NULL DEFAULT 0,
    cost_micros   BIGINT NOT NULL DEFAULT 0,
    latency_ms    INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_tasks_company ON ai_tasks (company_id, created_at DESC);
CREATE INDEX idx_ai_tasks_cache ON ai_tasks (kind, input_hash, created_at DESC);
-- The platform-wide daily budget sums every company's calls since midnight.
CREATE INDEX idx_ai_tasks_created ON ai_tasks (created_at);

-- ─── permissions ────────────────────────────────────────────────────
INSERT INTO permissions (code, resource, action, description) VALUES
    ('load_ads.create',        'load_ads', 'create',        'Create a load advertisement'),
    ('load_ads.view',          'load_ads', 'view',          'View load advertisements'),
    ('load_ads.delete',        'load_ads', 'delete',        'Archive a load advertisement'),
    ('load_ads.publish',       'load_ads', 'publish',       'Publish an advertisement to Telegram groups'),
    ('telegram.connect',       'telegram', 'connect',       'Connect or disconnect an own Telegram account'),
    ('telegram.groups.manage', 'telegram', 'groups_manage', 'Connect groups and edit their rules'),
    ('posting.view',           'posting',  'view',          'Read posting history'),
    ('dashboard.view',         'dashboard', 'view',         'Read the distribution dashboard')
ON CONFLICT (code) DO NOTHING;

-- Both company roles get the same codes; who may act on which account and whose
-- history they may read is scoped in the application, because the distinction
-- is ownership rather than capability.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('company_manager', 'dispatcher')
  AND p.code IN ('load_ads.create', 'load_ads.view', 'load_ads.delete', 'load_ads.publish',
                 'telegram.connect', 'telegram.groups.manage', 'posting.view', 'dashboard.view')
ON CONFLICT DO NOTHING;
