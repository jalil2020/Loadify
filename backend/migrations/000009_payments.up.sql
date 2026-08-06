-- 000009 · Subscriptions, wallets, double-entry ledger, escrow, invoices

CREATE TABLE subscription_plans (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code              VARCHAR(32) NOT NULL UNIQUE,
    name_uz           VARCHAR(100) NOT NULL,
    name_ru           VARCHAR(100) NOT NULL,
    name_en           VARCHAR(100) NOT NULL,
    description       TEXT,
    target_role       VARCHAR(16) NOT NULL CHECK (target_role IN ('driver', 'company')),
    price_amount      BIGINT NOT NULL CHECK (price_amount >= 0),
    currency          CHAR(3) NOT NULL DEFAULT 'UZS',
    period            VARCHAR(8) NOT NULL CHECK (period IN ('month', 'year')),
    trial_days        SMALLINT NOT NULL DEFAULT 0,
    features          JSONB NOT NULL DEFAULT '[]'::JSONB,
    limits            JSONB NOT NULL DEFAULT '{}'::JSONB,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    is_default        BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order        SMALLINT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subscription_plans_active ON subscription_plans (target_role, is_active, sort_order);
CREATE TRIGGER trg_subscription_plans_updated_at BEFORE UPDATE ON subscription_plans
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payment_orders (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID REFERENCES users (id) ON DELETE SET NULL,
    company_id        UUID REFERENCES companies (id) ON DELETE SET NULL,
    provider          VARCHAR(16) NOT NULL
                      CHECK (provider IN ('click', 'payme', 'uzum', 'stripe', 'wallet', 'manual')),
    provider_order_id VARCHAR(128),
    purpose           VARCHAR(16) NOT NULL
                      CHECK (purpose IN ('subscription', 'escrow', 'topup', 'other')),
    reference_type    VARCHAR(32),
    reference_id      UUID,
    amount            BIGINT NOT NULL CHECK (amount > 0),
    currency          CHAR(3) NOT NULL DEFAULT 'UZS',
    status            VARCHAR(16) NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'processing', 'paid', 'failed', 'cancelled', 'refunded')),
    idempotency_key   VARCHAR(128) NOT NULL,
    checkout_url      VARCHAR(1024),
    payload           JSONB NOT NULL DEFAULT '{}'::JSONB,
    failed_reason     TEXT,
    paid_at           TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_payment_orders_idempotency ON payment_orders (idempotency_key);
CREATE UNIQUE INDEX idx_payment_orders_provider_ref ON payment_orders (provider, provider_order_id)
    WHERE provider_order_id IS NOT NULL;
CREATE INDEX idx_payment_orders_user ON payment_orders (user_id, created_at DESC);
CREATE INDEX idx_payment_orders_company ON payment_orders (company_id, created_at DESC);
CREATE INDEX idx_payment_orders_status ON payment_orders (status, created_at)
    WHERE status IN ('pending', 'processing');
CREATE TRIGGER trg_payment_orders_updated_at BEFORE UPDATE ON payment_orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE subscriptions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID REFERENCES users (id) ON DELETE CASCADE,
    company_id       UUID REFERENCES companies (id) ON DELETE CASCADE,
    plan_id          UUID NOT NULL REFERENCES subscription_plans (id) ON DELETE RESTRICT,
    status           VARCHAR(16) NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'trialing', 'active', 'expired', 'cancelled')),
    started_at       TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ,
    auto_renew       BOOLEAN NOT NULL DEFAULT FALSE,
    payment_order_id UUID REFERENCES payment_orders (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_subscriptions_owner CHECK (
        (user_id IS NOT NULL AND company_id IS NULL) OR
        (user_id IS NULL AND company_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX idx_subscriptions_active_user ON subscriptions (user_id)
    WHERE status IN ('trialing', 'active') AND user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_subscriptions_active_company ON subscriptions (company_id)
    WHERE status IN ('trialing', 'active') AND company_id IS NOT NULL;
CREATE INDEX idx_subscriptions_expiry ON subscriptions (expires_at)
    WHERE status IN ('trialing', 'active');
CREATE TRIGGER trg_subscriptions_updated_at BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE wallets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type   VARCHAR(16) NOT NULL CHECK (owner_type IN ('user', 'company', 'platform')),
    owner_id     UUID,
    currency     CHAR(3) NOT NULL DEFAULT 'UZS',
    balance      BIGINT NOT NULL DEFAULT 0,
    held_balance BIGINT NOT NULL DEFAULT 0 CHECK (held_balance >= 0),
    status       VARCHAR(16) NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'frozen', 'closed')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_wallets_balance CHECK (balance >= held_balance OR owner_type = 'platform')
);
CREATE UNIQUE INDEX idx_wallets_owner ON wallets (owner_type, owner_id, currency)
    WHERE owner_id IS NOT NULL;
CREATE UNIQUE INDEX idx_wallets_platform ON wallets (currency)
    WHERE owner_type = 'platform';
CREATE TRIGGER trg_wallets_updated_at BEFORE UPDATE ON wallets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Double entry: every movement writes a debit row and a credit row sharing group_id.
CREATE TABLE wallet_transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id       UUID NOT NULL REFERENCES wallets (id) ON DELETE RESTRICT,
    group_id        UUID NOT NULL,
    direction       VARCHAR(8) NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount          BIGINT NOT NULL CHECK (amount > 0),
    balance_after   BIGINT NOT NULL,
    currency        CHAR(3) NOT NULL DEFAULT 'UZS',
    category        VARCHAR(24) NOT NULL
                    CHECK (category IN ('topup', 'payout', 'escrow_hold', 'escrow_release',
                                        'escrow_refund', 'commission', 'subscription',
                                        'refund', 'adjustment', 'penalty')),
    reference_type  VARCHAR(32),
    reference_id    UUID,
    description     VARCHAR(255),
    idempotency_key VARCHAR(128) NOT NULL,
    created_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_wallet_tx_idempotency ON wallet_transactions (idempotency_key);
CREATE INDEX idx_wallet_tx_wallet ON wallet_transactions (wallet_id, created_at DESC);
CREATE INDEX idx_wallet_tx_group ON wallet_transactions (group_id);
CREATE INDEX idx_wallet_tx_reference ON wallet_transactions (reference_type, reference_id);

CREATE TABLE escrows (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id           UUID NOT NULL REFERENCES trips (id) ON DELETE RESTRICT,
    payer_wallet_id   UUID NOT NULL REFERENCES wallets (id) ON DELETE RESTRICT,
    payee_wallet_id   UUID NOT NULL REFERENCES wallets (id) ON DELETE RESTRICT,
    amount            BIGINT NOT NULL CHECK (amount > 0),
    commission_amount BIGINT NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),
    currency          CHAR(3) NOT NULL DEFAULT 'UZS',
    status            VARCHAR(16) NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'funded', 'released', 'refunded', 'disputed')),
    funded_at         TIMESTAMPTZ,
    released_at       TIMESTAMPTZ,
    refunded_at       TIMESTAMPTZ,
    released_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_escrows_trip ON escrows (trip_id);
CREATE INDEX idx_escrows_status ON escrows (status, created_at);
CREATE TRIGGER trg_escrows_updated_at BEFORE UPDATE ON escrows
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payment_webhooks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        VARCHAR(16) NOT NULL,
    event_id        VARCHAR(128),
    event_type      VARCHAR(64),
    order_id        UUID REFERENCES payment_orders (id) ON DELETE SET NULL,
    signature_valid BOOLEAN NOT NULL DEFAULT FALSE,
    payload         JSONB NOT NULL,
    processed_at    TIMESTAMPTZ,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_payment_webhooks_event ON payment_webhooks (provider, event_id)
    WHERE event_id IS NOT NULL;
CREATE INDEX idx_payment_webhooks_order ON payment_webhooks (order_id);

CREATE TABLE invoices (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number     VARCHAR(32) NOT NULL DEFAULT next_reference('INV', 'invoice_number_seq'),
    company_id UUID REFERENCES companies (id) ON DELETE SET NULL,
    user_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    trip_id    UUID REFERENCES trips (id) ON DELETE SET NULL,
    amount     BIGINT NOT NULL CHECK (amount >= 0),
    tax_amount BIGINT NOT NULL DEFAULT 0,
    currency   CHAR(3) NOT NULL DEFAULT 'UZS',
    status     VARCHAR(16) NOT NULL DEFAULT 'issued'
               CHECK (status IN ('draft', 'issued', 'paid', 'overdue', 'cancelled')),
    items      JSONB NOT NULL DEFAULT '[]'::JSONB,
    file_id    UUID REFERENCES files (id) ON DELETE SET NULL,
    issued_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_at     TIMESTAMPTZ,
    paid_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_invoices_number ON invoices (number);
CREATE INDEX idx_invoices_company ON invoices (company_id, issued_at DESC);
CREATE TRIGGER trg_invoices_updated_at BEFORE UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE refunds (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_order_id UUID NOT NULL REFERENCES payment_orders (id) ON DELETE RESTRICT,
    amount           BIGINT NOT NULL CHECK (amount > 0),
    currency         CHAR(3) NOT NULL DEFAULT 'UZS',
    reason           TEXT NOT NULL,
    status           VARCHAR(16) NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'approved', 'rejected', 'processed', 'failed')),
    requested_by     UUID REFERENCES users (id) ON DELETE SET NULL,
    approved_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    processed_at     TIMESTAMPTZ,
    provider_ref     VARCHAR(128),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refunds_order ON refunds (payment_order_id);
CREATE INDEX idx_refunds_status ON refunds (status, created_at);
CREATE TRIGGER trg_refunds_updated_at BEFORE UPDATE ON refunds
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
