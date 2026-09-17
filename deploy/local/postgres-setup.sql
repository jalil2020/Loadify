-- A local PostgreSQL for running Loadify without Docker.
--
-- Run it once, as the superuser, after PostGIS is installed for this server:
--
--   "C:\Program Files\PostgreSQL\16\bin\psql.exe" -U postgres -f deploy/local/postgres-setup.sql
--
-- The extensions are created here because PostGIS can only be created by a
-- superuser and the application role should not be one. Migration 000001
-- finds them already there. The role and password match .env.example.
--
-- Running it again changes nothing.

\set ON_ERROR_STOP on

SELECT 'CREATE ROLE loadify LOGIN PASSWORD ''loadify_secret'''
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'loadify')
\gexec

SELECT 'CREATE DATABASE loadify OWNER loadify ENCODING ''UTF8'' TEMPLATE template0'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'loadify')
\gexec

\connect loadify

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS unaccent;

SELECT postgis_full_version();
