-- 000005 · Drivers, documents, vehicles, trailers, availability, location traces

CREATE TABLE drivers (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    company_id            UUID REFERENCES companies (id) ON DELETE SET NULL,
    license_number        VARCHAR(64),
    license_issued_at     DATE,
    license_expires_at    DATE,
    license_categories    TEXT[] NOT NULL DEFAULT '{}',
    experience_years      SMALLINT NOT NULL DEFAULT 0 CHECK (experience_years BETWEEN 0 AND 70),
    home_city_id          UUID REFERENCES cities (id) ON DELETE SET NULL,
    languages             TEXT[] NOT NULL DEFAULT '{uz}',
    bio                   TEXT,
    status                VARCHAR(16) NOT NULL DEFAULT 'offline'
                          CHECK (status IN ('offline', 'online', 'busy')),
    verification_status   VARCHAR(16) NOT NULL DEFAULT 'pending'
                          CHECK (verification_status IN ('pending', 'verified', 'rejected')),
    rejection_reason      TEXT,
    verified_at           TIMESTAMPTZ,
    verified_by           UUID REFERENCES users (id) ON DELETE SET NULL,
    rating                NUMERIC(3, 2) NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
    rating_count          INTEGER  NOT NULL DEFAULT 0,
    completed_trips       INTEGER  NOT NULL DEFAULT 0,
    cancelled_trips       INTEGER  NOT NULL DEFAULT 0,
    acceptance_rate       NUMERIC(5, 2) NOT NULL DEFAULT 0,
    avg_response_time_sec INTEGER  NOT NULL DEFAULT 0,
    total_distance_km     NUMERIC(12, 2) NOT NULL DEFAULT 0,
    current_location      geography(Point, 4326),
    location_updated_at   TIMESTAMPTZ,
    last_online_at        TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_drivers_user ON drivers (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_drivers_status ON drivers (status) WHERE deleted_at IS NULL;
CREATE INDEX idx_drivers_verification ON drivers (verification_status, created_at)
    WHERE verification_status = 'pending' AND deleted_at IS NULL;
CREATE INDEX idx_drivers_location_online ON drivers USING GIST (current_location)
    WHERE status IN ('online', 'busy') AND deleted_at IS NULL;
CREATE INDEX idx_drivers_company ON drivers (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX idx_drivers_rating ON drivers (rating DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_drivers_updated_at BEFORE UPDATE ON drivers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE driver_documents (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id        UUID NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    type             VARCHAR(32) NOT NULL
                     CHECK (type IN ('driving_license', 'passport', 'id_card',
                                     'insurance', 'tech_passport', 'medical', 'other')),
    side             VARCHAR(8)  NOT NULL DEFAULT 'single'
                     CHECK (side IN ('front', 'back', 'single')),
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
CREATE UNIQUE INDEX idx_driver_documents_unique
    ON driver_documents (driver_id, type, side) WHERE deleted_at IS NULL;
CREATE INDEX idx_driver_documents_review ON driver_documents (status, created_at)
    WHERE status = 'pending' AND deleted_at IS NULL;
CREATE INDEX idx_driver_documents_expiry ON driver_documents (expires_at)
    WHERE status = 'approved' AND expires_at IS NOT NULL;
CREATE TRIGGER trg_driver_documents_updated_at BEFORE UPDATE ON driver_documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE vehicles (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id                  UUID NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    company_id                 UUID REFERENCES companies (id) ON DELETE SET NULL,
    type                       VARCHAR(24) NOT NULL
                               CHECK (type IN ('tent', 'refrigerator', 'isothermal', 'flatbed',
                                               'container', 'tanker', 'car_carrier', 'grain',
                                               'dump', 'van', 'lowboy', 'other')),
    make                       VARCHAR(64) NOT NULL,
    model                      VARCHAR(64) NOT NULL,
    year                       SMALLINT CHECK (year BETWEEN 1950 AND 2100),
    plate_number               VARCHAR(24) NOT NULL,
    vin                        VARCHAR(32),
    color                      VARCHAR(32),
    capacity_kg                INTEGER NOT NULL CHECK (capacity_kg > 0),
    volume_m3                  NUMERIC(8, 2) CHECK (volume_m3 IS NULL OR volume_m3 > 0),
    length_cm                  INTEGER CHECK (length_cm IS NULL OR length_cm > 0),
    width_cm                   INTEGER CHECK (width_cm IS NULL OR width_cm > 0),
    height_cm                  INTEGER CHECK (height_cm IS NULL OR height_cm > 0),
    fuel_type                  VARCHAR(16) NOT NULL DEFAULT 'diesel'
                               CHECK (fuel_type IN ('diesel', 'petrol', 'gas', 'methane',
                                                    'propane', 'electric', 'hybrid')),
    axles                      SMALLINT,
    has_adr                    BOOLEAN NOT NULL DEFAULT FALSE,
    has_refrigeration          BOOLEAN NOT NULL DEFAULT FALSE,
    temp_min_c                 SMALLINT,
    temp_max_c                 SMALLINT,
    has_tail_lift              BOOLEAN NOT NULL DEFAULT FALSE,
    has_gps                    BOOLEAN NOT NULL DEFAULT FALSE,
    insurance_expires_at       DATE,
    tech_inspection_expires_at DATE,
    status                     VARCHAR(16) NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending', 'approved', 'rejected', 'inactive')),
    rejection_reason           TEXT,
    is_primary                 BOOLEAN NOT NULL DEFAULT FALSE,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                 TIMESTAMPTZ,
    CONSTRAINT chk_vehicles_temp_range CHECK (temp_min_c IS NULL OR temp_max_c IS NULL OR temp_min_c <= temp_max_c)
);
CREATE UNIQUE INDEX idx_vehicles_plate ON vehicles (upper(plate_number)) WHERE deleted_at IS NULL;
CREATE INDEX idx_vehicles_driver ON vehicles (driver_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_vehicles_company ON vehicles (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX idx_vehicles_search ON vehicles (type, capacity_kg) WHERE status = 'approved' AND deleted_at IS NULL;
CREATE UNIQUE INDEX idx_vehicles_one_primary ON vehicles (driver_id)
    WHERE is_primary AND deleted_at IS NULL;
CREATE TRIGGER trg_vehicles_updated_at BEFORE UPDATE ON vehicles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE vehicle_photos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id UUID NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    file_id    UUID NOT NULL REFERENCES files (id) ON DELETE RESTRICT,
    position   VARCHAR(16) NOT NULL DEFAULT 'other'
               CHECK (position IN ('front', 'back', 'left', 'right', 'inside', 'other')),
    sort_order SMALLINT    NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_vehicle_photos_vehicle ON vehicle_photos (vehicle_id, sort_order);

CREATE TABLE trailers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id   UUID NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    type         VARCHAR(24) NOT NULL,
    plate_number VARCHAR(24) NOT NULL,
    make         VARCHAR(64),
    model        VARCHAR(64),
    year         SMALLINT,
    capacity_kg  INTEGER CHECK (capacity_kg IS NULL OR capacity_kg > 0),
    volume_m3    NUMERIC(8, 2),
    length_cm    INTEGER,
    width_cm     INTEGER,
    height_cm    INTEGER,
    axles        SMALLINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX idx_trailers_vehicle ON trailers (vehicle_id) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_trailers_updated_at BEFORE UPDATE ON trailers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE driver_availability (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id            UUID NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    vehicle_id           UUID REFERENCES vehicles (id) ON DELETE SET NULL,
    from_address_id      UUID REFERENCES addresses (id) ON DELETE SET NULL,
    available_from       TIMESTAMPTZ NOT NULL,
    available_to         TIMESTAMPTZ,
    preferred_region_ids UUID[] NOT NULL DEFAULT '{}',
    max_distance_km      INTEGER,
    min_price_amount     BIGINT,
    currency             CHAR(3) NOT NULL DEFAULT 'UZS',
    notes                TEXT,
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_availability_window CHECK (available_to IS NULL OR available_to > available_from)
);
CREATE INDEX idx_driver_availability_active
    ON driver_availability (driver_id, is_active, available_from);
CREATE INDEX idx_driver_availability_regions
    ON driver_availability USING GIN (preferred_region_ids);
CREATE TRIGGER trg_driver_availability_updated_at BEFORE UPDATE ON driver_availability
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE driver_preferences (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id              UUID NOT NULL UNIQUE REFERENCES drivers (id) ON DELETE CASCADE,
    max_distance_km        INTEGER NOT NULL DEFAULT 500,
    preferred_cargo_types  UUID[]  NOT NULL DEFAULT '{}',
    preferred_region_ids   UUID[]  NOT NULL DEFAULT '{}',
    min_price_amount       BIGINT,
    notify_new_loads       BOOLEAN NOT NULL DEFAULT TRUE,
    notify_radius_km       INTEGER NOT NULL DEFAULT 100,
    auto_accept_offers     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_driver_preferences_updated_at BEFORE UPDATE ON driver_preferences
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─── location traces (monthly range partitions) ─────────────────────
-- driver_id and trip_id carry no foreign keys on purpose: this is the highest
-- write-volume table in the system and FK checks on every buffered batch insert
-- are not worth the referential guarantee. Orphans are swept by the retention job.
CREATE TABLE driver_locations (
    id            BIGSERIAL,
    driver_id     UUID NOT NULL,
    trip_id       UUID,
    location      geography(Point, 4326) NOT NULL,
    speed_kmh     NUMERIC(6, 2),
    heading       NUMERIC(6, 2),
    accuracy_m    NUMERIC(8, 2),
    altitude_m    NUMERIC(8, 2),
    battery_level SMALLINT CHECK (battery_level IS NULL OR battery_level BETWEEN 0 AND 100),
    is_moving     BOOLEAN NOT NULL DEFAULT TRUE,
    recorded_at   TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, recorded_at)
) PARTITION BY RANGE (recorded_at);

CREATE INDEX idx_driver_locations_driver ON driver_locations (driver_id, recorded_at DESC);
CREATE INDEX idx_driver_locations_trip ON driver_locations (trip_id, recorded_at)
    WHERE trip_id IS NOT NULL;
CREATE INDEX idx_driver_locations_geo ON driver_locations USING GIST (location);

-- Bootstrap the current month and the next three so writes never fail
-- before the maintenance job runs.
DO $$
DECLARE d DATE := date_trunc('month', now())::DATE;
BEGIN
    FOR i IN 0..3 LOOP
        PERFORM ensure_month_partition('driver_locations', (d + (i || ' month')::INTERVAL)::DATE);
    END LOOP;
END $$;
