-- 000001 · Extensions and shared helpers

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- Keeps updated_at honest regardless of which client writes the row.
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Human-readable sequential references: LD-2601-00042, TR-2601-00042.
CREATE SEQUENCE IF NOT EXISTS load_reference_seq START 1;
CREATE SEQUENCE IF NOT EXISTS trip_number_seq START 1;
CREATE SEQUENCE IF NOT EXISTS invoice_number_seq START 1;

CREATE OR REPLACE FUNCTION next_reference(prefix TEXT, seq TEXT) RETURNS TEXT AS $$
BEGIN
    RETURN prefix || '-' || to_char(now(), 'YYMM') || '-'
           || lpad(nextval(seq)::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- Creates the monthly partition covering the given day, if missing.
CREATE OR REPLACE FUNCTION ensure_month_partition(parent TEXT, day DATE) RETURNS VOID AS $$
DECLARE
    start_ts DATE := date_trunc('month', day)::DATE;
    end_ts   DATE := (date_trunc('month', day) + INTERVAL '1 month')::DATE;
    part     TEXT := parent || '_' || to_char(start_ts, 'YYYY_MM');
BEGIN
    IF to_regclass(part) IS NULL THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
            part, parent, start_ts, end_ts);
    END IF;
END;
$$ LANGUAGE plpgsql;
