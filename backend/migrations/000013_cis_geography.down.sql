-- Cities cascade with their regions. An advert that pointed at one keeps its
-- typed text: load_ads.origin_city_id is ON DELETE SET NULL.
DELETE FROM regions
WHERE code LIKE 'RU-%' OR code LIKE 'KZ-%' OR code LIKE 'KG-%'
   OR code LIKE 'TJ-%' OR code LIKE 'TM-%' OR code LIKE 'AF-%'
   OR code LIKE 'TR-%' OR code LIKE 'CN-%';
