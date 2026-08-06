-- 000003 · Geography reference data and addresses

CREATE TABLE countries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          CHAR(2)     NOT NULL UNIQUE,
    code3         CHAR(3)     NOT NULL,
    name_uz       VARCHAR(100) NOT NULL,
    name_ru       VARCHAR(100) NOT NULL,
    name_en       VARCHAR(100) NOT NULL,
    phone_code    VARCHAR(8)   NOT NULL,
    currency_code CHAR(3)      NOT NULL,
    flag_emoji    VARCHAR(16),
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    sort_order    SMALLINT     NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_countries_updated_at BEFORE UPDATE ON countries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE regions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country_id UUID NOT NULL REFERENCES countries (id) ON DELETE CASCADE,
    code       VARCHAR(16)  NOT NULL,
    name_uz    VARCHAR(100) NOT NULL,
    name_ru    VARCHAR(100) NOT NULL,
    name_en    VARCHAR(100) NOT NULL,
    center     geography(Point, 4326),
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    sort_order SMALLINT     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (country_id, code)
);
CREATE INDEX idx_regions_country ON regions (country_id);
CREATE INDEX idx_regions_center ON regions USING GIST (center);
CREATE TRIGGER trg_regions_updated_at BEFORE UPDATE ON regions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    region_id   UUID NOT NULL REFERENCES regions (id) ON DELETE CASCADE,
    name_uz     VARCHAR(100) NOT NULL,
    name_ru     VARCHAR(100) NOT NULL,
    name_en     VARCHAR(100) NOT NULL,
    location    geography(Point, 4326),
    is_capital  BOOLEAN      NOT NULL DEFAULT FALSE,
    population  INTEGER,
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_cities_region ON cities (region_id);
CREATE INDEX idx_cities_location ON cities USING GIST (location);
CREATE INDEX idx_cities_name_trgm ON cities USING GIN (name_uz gin_trgm_ops, name_ru gin_trgm_ops);
CREATE TRIGGER trg_cities_updated_at BEFORE UPDATE ON cities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE addresses (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country_id      UUID REFERENCES countries (id) ON DELETE SET NULL,
    region_id       UUID REFERENCES regions (id) ON DELETE SET NULL,
    city_id         UUID REFERENCES cities (id) ON DELETE SET NULL,
    line1           VARCHAR(255) NOT NULL DEFAULT '',
    line2           VARCHAR(255),
    postal_code     VARCHAR(20),
    formatted       VARCHAR(512) NOT NULL DEFAULT '',
    google_place_id VARCHAR(255),
    location        geography(Point, 4326) NOT NULL,
    contact_name    VARCHAR(120),
    contact_phone   VARCHAR(20),
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_addresses_location ON addresses USING GIST (location);
CREATE INDEX idx_addresses_region ON addresses (region_id);
CREATE INDEX idx_addresses_city ON addresses (city_id);
CREATE INDEX idx_addresses_place_id ON addresses (google_place_id) WHERE google_place_id IS NOT NULL;
CREATE TRIGGER trg_addresses_updated_at BEFORE UPDATE ON addresses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
