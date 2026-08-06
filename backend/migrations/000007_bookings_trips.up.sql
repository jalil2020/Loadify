-- 000007 · Bookings, trips, trip events, documents, ratings

CREATE TABLE bookings (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    load_id              UUID NOT NULL REFERENCES loads (id) ON DELETE CASCADE,
    driver_id            UUID NOT NULL REFERENCES drivers (id) ON DELETE CASCADE,
    vehicle_id           UUID REFERENCES vehicles (id) ON DELETE SET NULL,
    company_id           UUID NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    initiated_by         VARCHAR(8) NOT NULL CHECK (initiated_by IN ('driver', 'company')),
    status               VARCHAR(16) NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'offered', 'accepted', 'rejected',
                                           'withdrawn', 'expired', 'cancelled')),
    offered_price_amount BIGINT CHECK (offered_price_amount IS NULL OR offered_price_amount >= 0),
    agreed_price_amount  BIGINT CHECK (agreed_price_amount IS NULL OR agreed_price_amount >= 0),
    currency             CHAR(3) NOT NULL DEFAULT 'UZS',
    message              TEXT,
    response_message     TEXT,
    expires_at           TIMESTAMPTZ,
    responded_at         TIMESTAMPTZ,
    responded_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    cancelled_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    cancellation_reason  TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ
);
-- A driver may hold at most one live booking per load.
CREATE UNIQUE INDEX idx_bookings_one_live
    ON bookings (load_id, driver_id)
    WHERE status IN ('pending', 'offered', 'accepted') AND deleted_at IS NULL;
CREATE INDEX idx_bookings_load ON bookings (load_id, status);
CREATE INDEX idx_bookings_driver ON bookings (driver_id, status, created_at DESC);
CREATE INDEX idx_bookings_company ON bookings (company_id, status, created_at DESC);
CREATE INDEX idx_bookings_expiry ON bookings (expires_at)
    WHERE status IN ('pending', 'offered') AND expires_at IS NOT NULL;
CREATE TRIGGER trg_bookings_updated_at BEFORE UPDATE ON bookings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE trips (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id           UUID NOT NULL REFERENCES bookings (id) ON DELETE RESTRICT,
    load_id              UUID NOT NULL REFERENCES loads (id) ON DELETE RESTRICT,
    driver_id            UUID NOT NULL REFERENCES drivers (id) ON DELETE RESTRICT,
    vehicle_id           UUID REFERENCES vehicles (id) ON DELETE SET NULL,
    company_id           UUID NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    trip_number          VARCHAR(24) NOT NULL DEFAULT next_reference('TR', 'trip_number_seq'),
    status               VARCHAR(24) NOT NULL DEFAULT 'created'
                         CHECK (status IN ('created', 'en_route_to_pickup', 'at_pickup', 'loaded',
                                           'in_transit', 'at_delivery', 'delivered', 'completed',
                                           'cancelled', 'disputed')),
    agreed_price_amount  BIGINT NOT NULL CHECK (agreed_price_amount >= 0),
    commission_amount    BIGINT NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),
    currency             CHAR(3) NOT NULL DEFAULT 'UZS',
    distance_planned_km  NUMERIC(10, 2),
    distance_actual_km   NUMERIC(10, 2),
    eta_at               TIMESTAMPTZ,
    started_at           TIMESTAMPTZ,
    arrived_pickup_at    TIMESTAMPTZ,
    loaded_at            TIMESTAMPTZ,
    arrived_delivery_at  TIMESTAMPTZ,
    delivered_at         TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    cancelled_at         TIMESTAMPTZ,
    cancelled_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    cancellation_reason  TEXT,
    notes                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_trips_booking ON trips (booking_id);
CREATE UNIQUE INDEX idx_trips_number ON trips (trip_number);
CREATE INDEX idx_trips_driver ON trips (driver_id, status, created_at DESC);
CREATE INDEX idx_trips_company ON trips (company_id, status, created_at DESC);
CREATE INDEX idx_trips_load ON trips (load_id);
CREATE INDEX idx_trips_active ON trips (status)
    WHERE status IN ('en_route_to_pickup', 'at_pickup', 'loaded', 'in_transit', 'at_delivery');
CREATE TRIGGER trg_trips_updated_at BEFORE UPDATE ON trips
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE trip_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id       UUID NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    type          VARCHAR(24) NOT NULL
                  CHECK (type IN ('status_change', 'location', 'document', 'note', 'delay', 'issue')),
    from_status   VARCHAR(24),
    to_status     VARCHAR(24),
    location      geography(Point, 4326),
    payload       JSONB NOT NULL DEFAULT '{}'::JSONB,
    actor_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trip_events_trip ON trip_events (trip_id, created_at DESC);
CREATE INDEX idx_trip_events_type ON trip_events (type, created_at DESC);

CREATE TABLE trip_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id     UUID NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    file_id     UUID NOT NULL REFERENCES files (id) ON DELETE RESTRICT,
    type        VARCHAR(24) NOT NULL
                CHECK (type IN ('cmr', 'invoice', 'waybill', 'pod', 'photo', 'signature', 'other')),
    name        VARCHAR(255),
    uploaded_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX idx_trip_documents_trip ON trip_documents (trip_id, type) WHERE deleted_at IS NULL;

CREATE TABLE ratings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id       UUID NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    rater_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ratee_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ratee_type    VARCHAR(8) NOT NULL CHECK (ratee_type IN ('driver', 'company')),
    driver_id     UUID REFERENCES drivers (id) ON DELETE CASCADE,
    company_id    UUID REFERENCES companies (id) ON DELETE CASCADE,
    score         SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 5),
    comment       TEXT,
    criteria      JSONB NOT NULL DEFAULT '{}'::JSONB,
    is_hidden     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_ratings_unique ON ratings (trip_id, rater_user_id);
CREATE INDEX idx_ratings_driver ON ratings (driver_id, created_at DESC) WHERE driver_id IS NOT NULL;
CREATE INDEX idx_ratings_company ON ratings (company_id, created_at DESC) WHERE company_id IS NOT NULL;
CREATE TRIGGER trg_ratings_updated_at BEFORE UPDATE ON ratings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
