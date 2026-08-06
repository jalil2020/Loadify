-- 000006 · Cargo types, loads, attachments, saved searches

CREATE TABLE cargo_types (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                 VARCHAR(32) NOT NULL UNIQUE,
    name_uz              VARCHAR(100) NOT NULL,
    name_ru              VARCHAR(100) NOT NULL,
    name_en              VARCHAR(100) NOT NULL,
    icon                 VARCHAR(64),
    requires_adr         BOOLEAN NOT NULL DEFAULT FALSE,
    requires_temperature BOOLEAN NOT NULL DEFAULT FALSE,
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order           SMALLINT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_cargo_types_updated_at BEFORE UPDATE ON cargo_types
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE loads (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id             UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    created_by_user_id     UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    reference_no           VARCHAR(24) NOT NULL DEFAULT next_reference('LD', 'load_reference_seq'),
    title                  VARCHAR(255) NOT NULL,
    description            TEXT,
    cargo_type_id          UUID REFERENCES cargo_types (id) ON DELETE SET NULL,

    weight_kg              INTEGER NOT NULL CHECK (weight_kg > 0),
    volume_m3              NUMERIC(8, 2) CHECK (volume_m3 IS NULL OR volume_m3 > 0),
    length_cm              INTEGER,
    width_cm               INTEGER,
    height_cm              INTEGER,
    packaging              VARCHAR(16) NOT NULL DEFAULT 'other'
                           CHECK (packaging IN ('pallet', 'box', 'bag', 'bulk', 'roll', 'barrel', 'container', 'other')),
    quantity               INTEGER,

    pickup_address_id      UUID NOT NULL REFERENCES addresses (id) ON DELETE RESTRICT,
    delivery_address_id    UUID NOT NULL REFERENCES addresses (id) ON DELETE RESTRICT,
    pickup_location        geography(Point, 4326) NOT NULL,
    delivery_location      geography(Point, 4326) NOT NULL,
    pickup_region_id       UUID REFERENCES regions (id) ON DELETE SET NULL,
    delivery_region_id     UUID REFERENCES regions (id) ON DELETE SET NULL,
    pickup_from            TIMESTAMPTZ NOT NULL,
    pickup_to              TIMESTAMPTZ,
    delivery_from          TIMESTAMPTZ,
    delivery_to            TIMESTAMPTZ,
    distance_km            NUMERIC(10, 2),

    required_vehicle_type  VARCHAR(24),
    required_capacity_kg   INTEGER,
    requires_adr           BOOLEAN NOT NULL DEFAULT FALSE,
    requires_refrigeration BOOLEAN NOT NULL DEFAULT FALSE,
    temp_min_c             SMALLINT,
    temp_max_c             SMALLINT,
    requires_tail_lift     BOOLEAN NOT NULL DEFAULT FALSE,
    loading_type           VARCHAR(16) NOT NULL DEFAULT 'rear'
                           CHECK (loading_type IN ('rear', 'side', 'top', 'full')),
    special_requirements   TEXT,

    price_amount           BIGINT NOT NULL CHECK (price_amount >= 0),
    price_currency         CHAR(3) NOT NULL DEFAULT 'UZS',
    price_type             VARCHAR(12) NOT NULL DEFAULT 'fixed'
                           CHECK (price_type IN ('fixed', 'negotiable', 'per_km')),
    payment_terms          VARCHAR(16) NOT NULL DEFAULT 'on_delivery'
                           CHECK (payment_terms IN ('on_loading', 'on_delivery', 'prepayment', 'deferred')),
    payment_method         VARCHAR(16) NOT NULL DEFAULT 'cash'
                           CHECK (payment_method IN ('cash', 'transfer', 'card', 'escrow')),
    prepayment_percent     SMALLINT CHECK (prepayment_percent IS NULL OR prepayment_percent BETWEEN 0 AND 100),
    deferred_days          SMALLINT,

    status                 VARCHAR(16) NOT NULL DEFAULT 'draft'
                           CHECK (status IN ('draft', 'published', 'reserved', 'assigned',
                                             'in_transit', 'delivered', 'completed',
                                             'cancelled', 'expired')),
    published_at           TIMESTAMPTZ,
    expires_at             TIMESTAMPTZ,
    cancelled_at           TIMESTAMPTZ,
    cancellation_reason    TEXT,

    views_count            INTEGER NOT NULL DEFAULT 0,
    reservations_count     INTEGER NOT NULL DEFAULT 0,
    is_featured            BOOLEAN NOT NULL DEFAULT FALSE,
    is_urgent              BOOLEAN NOT NULL DEFAULT FALSE,
    duplicate_hash         CHAR(64),
    fraud_score            NUMERIC(5, 2) NOT NULL DEFAULT 0,
    search_vector          TSVECTOR,

    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at             TIMESTAMPTZ,

    CONSTRAINT chk_loads_pickup_window CHECK (pickup_to IS NULL OR pickup_to >= pickup_from),
    CONSTRAINT chk_loads_delivery_window CHECK (delivery_to IS NULL OR delivery_from IS NULL OR delivery_to >= delivery_from),
    CONSTRAINT chk_loads_temp_range CHECK (temp_min_c IS NULL OR temp_max_c IS NULL OR temp_min_c <= temp_max_c)
);

CREATE UNIQUE INDEX idx_loads_reference ON loads (reference_no);
CREATE INDEX idx_loads_feed ON loads (status, published_at DESC)
    WHERE status = 'published' AND deleted_at IS NULL;
CREATE INDEX idx_loads_company ON loads (company_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_loads_creator ON loads (created_by_user_id, created_at DESC);
CREATE INDEX idx_loads_pickup_geo ON loads USING GIST (pickup_location)
    WHERE status = 'published' AND deleted_at IS NULL;
CREATE INDEX idx_loads_delivery_geo ON loads USING GIST (delivery_location)
    WHERE status = 'published' AND deleted_at IS NULL;
CREATE INDEX idx_loads_regions ON loads (pickup_region_id, delivery_region_id, status)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_loads_filters ON loads (required_vehicle_type, weight_kg, pickup_from)
    WHERE status = 'published' AND deleted_at IS NULL;
CREATE INDEX idx_loads_search_vector ON loads USING GIN (search_vector);
CREATE INDEX idx_loads_expiry ON loads (expires_at) WHERE status = 'published' AND deleted_at IS NULL;
CREATE INDEX idx_loads_duplicate_hash ON loads (duplicate_hash) WHERE duplicate_hash IS NOT NULL;
CREATE TRIGGER trg_loads_updated_at BEFORE UPDATE ON loads
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE FUNCTION loads_refresh_search_vector() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('simple', unaccent(coalesce(NEW.title, ''))), 'A') ||
        setweight(to_tsvector('simple', unaccent(coalesce(NEW.reference_no, ''))), 'A') ||
        setweight(to_tsvector('simple', unaccent(coalesce(NEW.description, ''))), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_loads_search_vector BEFORE INSERT OR UPDATE OF title, description, reference_no
    ON loads FOR EACH ROW EXECUTE FUNCTION loads_refresh_search_vector();

CREATE TABLE load_photos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    load_id    UUID NOT NULL REFERENCES loads (id) ON DELETE CASCADE,
    file_id    UUID NOT NULL REFERENCES files (id) ON DELETE RESTRICT,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_load_photos_load ON load_photos (load_id, sort_order);

CREATE TABLE load_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    load_id     UUID NOT NULL REFERENCES loads (id) ON DELETE CASCADE,
    file_id     UUID NOT NULL REFERENCES files (id) ON DELETE RESTRICT,
    type        VARCHAR(32) NOT NULL DEFAULT 'other'
                CHECK (type IN ('invoice', 'packing_list', 'certificate', 'permit', 'other')),
    name        VARCHAR(255),
    uploaded_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_load_documents_load ON load_documents (load_id);

CREATE TABLE load_views (
    id        BIGSERIAL PRIMARY KEY,
    load_id   UUID NOT NULL REFERENCES loads (id) ON DELETE CASCADE,
    user_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    ip_address INET,
    viewed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_load_views_load ON load_views (load_id, viewed_at DESC);
CREATE UNIQUE INDEX idx_load_views_unique_user ON load_views (load_id, user_id)
    WHERE user_id IS NOT NULL;

CREATE TABLE saved_loads (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    load_id    UUID NOT NULL REFERENCES loads (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, load_id)
);
CREATE INDEX idx_saved_loads_user ON saved_loads (user_id, created_at DESC);

CREATE TABLE saved_searches (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name             VARCHAR(100) NOT NULL,
    filters          JSONB NOT NULL DEFAULT '{}'::JSONB,
    notify_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    last_notified_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_saved_searches_user ON saved_searches (user_id);
CREATE INDEX idx_saved_searches_notify ON saved_searches (notify_enabled) WHERE notify_enabled;
CREATE INDEX idx_saved_searches_filters ON saved_searches USING GIN (filters);
CREATE TRIGGER trg_saved_searches_updated_at BEFORE UPDATE ON saved_searches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
