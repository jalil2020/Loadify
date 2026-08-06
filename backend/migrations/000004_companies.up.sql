-- 000004 · Logistics companies, members, verification documents

CREATE TABLE companies (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id       UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    legal_name          VARCHAR(255) NOT NULL,
    brand_name          VARCHAR(255) NOT NULL,
    tin                 VARCHAR(20),
    registration_number VARCHAR(64),
    description         TEXT,
    logo_file_id        UUID REFERENCES files (id) ON DELETE SET NULL,
    address_id          UUID REFERENCES addresses (id) ON DELETE SET NULL,
    email               VARCHAR(255),
    phone               VARCHAR(20)  NOT NULL,
    website             VARCHAR(255),
    employees_count     INTEGER,
    fleet_size          INTEGER,
    founded_year        SMALLINT,
    status              VARCHAR(16)  NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'verified', 'rejected', 'blocked')),
    rejection_reason    TEXT,
    verified_at         TIMESTAMPTZ,
    verified_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    rating              NUMERIC(3, 2) NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
    rating_count        INTEGER      NOT NULL DEFAULT 0,
    completed_trips     INTEGER      NOT NULL DEFAULT 0,
    cancelled_trips     INTEGER      NOT NULL DEFAULT 0,
    published_loads     INTEGER      NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_companies_tin ON companies (tin) WHERE tin IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_companies_owner ON companies (owner_user_id);
CREATE INDEX idx_companies_status ON companies (status) WHERE deleted_at IS NULL;
CREATE INDEX idx_companies_rating ON companies (rating DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_companies_name_trgm ON companies USING GIN (brand_name gin_trgm_ops);
CREATE TRIGGER trg_companies_updated_at BEFORE UPDATE ON companies
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE user_roles ADD CONSTRAINT fk_user_roles_company
    FOREIGN KEY (company_id) REFERENCES companies (id) ON DELETE CASCADE;

CREATE TABLE company_members (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    member_role VARCHAR(24) NOT NULL DEFAULT 'dispatcher'
                CHECK (member_role IN ('company_manager', 'dispatcher')),
    position    VARCHAR(100),
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    invited_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    invited_at  TIMESTAMPTZ,
    joined_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_company_members_unique
    ON company_members (company_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_company_members_user ON company_members (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_company_members_active
    ON company_members (company_id, is_active) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_company_members_updated_at BEFORE UPDATE ON company_members
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE company_documents (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    type             VARCHAR(32) NOT NULL
                     CHECK (type IN ('registration', 'license', 'tax_certificate', 'bank_details', 'other')),
    file_id          UUID NOT NULL REFERENCES files (id) ON DELETE RESTRICT,
    number           VARCHAR(64),
    issued_at        DATE,
    expires_at       DATE,
    status           VARCHAR(16) NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'approved', 'rejected')),
    rejection_reason TEXT,
    reviewed_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at       TIMESTAMPTZ
);
CREATE INDEX idx_company_documents_company ON company_documents (company_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_company_documents_review ON company_documents (status, created_at)
    WHERE status = 'pending' AND deleted_at IS NULL;
CREATE TRIGGER trg_company_documents_updated_at BEFORE UPDATE ON company_documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
