DROP TABLE IF EXISTS saved_searches;
DROP TABLE IF EXISTS saved_loads;
DROP TABLE IF EXISTS load_views;
DROP TABLE IF EXISTS load_documents;
DROP TABLE IF EXISTS load_photos;
DROP TRIGGER IF EXISTS trg_loads_search_vector ON loads;
DROP FUNCTION IF EXISTS loads_refresh_search_vector();
DROP TABLE IF EXISTS loads;
DROP TABLE IF EXISTS cargo_types;
