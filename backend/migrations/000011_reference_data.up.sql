-- 000011 · Reference data: roles, permissions, geography, cargo types, plans, settings
-- Idempotent: safe to re-run.

-- ─── roles ──────────────────────────────────────────────────────────
INSERT INTO roles (code, name, description, level, is_system) VALUES
    ('guest',           'Guest',            'Unauthenticated visitor',                 0, TRUE),
    ('driver',          'Driver',           'Truck driver / owner-operator',           10, TRUE),
    ('dispatcher',      'Dispatcher',       'Company dispatcher, scoped to a company', 20, TRUE),
    ('company_manager', 'Company Manager',  'Company owner or manager',                30, TRUE),
    ('admin',           'Administrator',    'Platform operations',                     80, TRUE),
    ('super_admin',     'Super Admin',      'Full platform control',                   100, TRUE)
ON CONFLICT (code) DO NOTHING;

-- ─── permissions ────────────────────────────────────────────────────
INSERT INTO permissions (code, resource, action, description) VALUES
    ('profile.read',            'profile',      'read',    'Read own profile'),
    ('profile.update',          'profile',      'update',  'Update own profile'),

    ('drivers.read',            'drivers',      'read',    'View driver profiles'),
    ('drivers.update',          'drivers',      'update',  'Update own driver profile'),
    ('drivers.verify',          'drivers',      'verify',  'Approve or reject driver documents'),
    ('drivers.block',           'drivers',      'block',   'Block a driver'),
    ('drivers.search',          'drivers',      'search',  'Search drivers and see the live map'),

    ('vehicles.create',         'vehicles',     'create',  'Add a vehicle'),
    ('vehicles.update',         'vehicles',     'update',  'Update a vehicle'),
    ('vehicles.delete',         'vehicles',     'delete',  'Remove a vehicle'),
    ('vehicles.verify',         'vehicles',     'verify',  'Approve or reject a vehicle'),

    ('companies.read',          'companies',    'read',    'View company profiles'),
    ('companies.update',        'companies',    'update',  'Update own company'),
    ('companies.verify',        'companies',    'verify',  'Approve or reject a company'),
    ('companies.block',         'companies',    'block',   'Block a company'),
    ('companies.members',       'companies',    'members', 'Manage company members'),

    ('loads.read',              'loads',        'read',    'View loads'),
    ('loads.create',            'loads',        'create',  'Create loads'),
    ('loads.update',            'loads',        'update',  'Update loads'),
    ('loads.delete',            'loads',        'delete',  'Delete loads'),
    ('loads.publish',           'loads',        'publish', 'Publish and unpublish loads'),
    ('loads.moderate',          'loads',        'moderate','Moderate any load'),

    ('bookings.create',         'bookings',     'create',  'Reserve a load or offer to a driver'),
    ('bookings.respond',        'bookings',     'respond', 'Accept or reject bookings'),
    ('bookings.cancel',         'bookings',     'cancel',  'Cancel a booking'),
    ('bookings.read',           'bookings',     'read',    'View bookings'),

    ('trips.read',              'trips',        'read',    'View trips'),
    ('trips.update',            'trips',        'update',  'Advance trip status'),
    ('trips.track',             'trips',        'track',   'Track trips live'),
    ('trips.cancel',            'trips',        'cancel',  'Cancel a trip'),

    ('chat.read',               'chat',         'read',    'Read chat messages'),
    ('chat.write',              'chat',         'write',   'Send chat messages'),

    ('payments.read',           'payments',     'read',    'View payments and wallet'),
    ('payments.manage',         'payments',     'manage',  'Top up, pay out, subscribe'),
    ('payments.refund',         'payments',     'refund',  'Issue refunds'),
    ('payments.escrow',         'payments',     'escrow',  'Fund and release escrow'),

    ('complaints.create',       'complaints',   'create',  'File a complaint'),
    ('complaints.manage',       'complaints',   'manage',  'Review and resolve complaints'),

    ('admin.dashboard',         'admin',        'read',    'Access the admin dashboard'),
    ('admin.users',             'admin',        'users',   'Manage users'),
    ('admin.reports',           'admin',        'reports', 'View reports and analytics'),
    ('admin.notifications',     'admin',        'notify',  'Broadcast notifications'),
    ('admin.settings',          'admin',        'settings','Change system settings'),
    ('admin.roles',             'admin',        'roles',   'Manage roles and permissions'),
    ('admin.audit',             'admin',        'audit',   'Read audit logs'),
    ('admin.cms',               'admin',        'cms',     'Manage CMS content')
ON CONFLICT (code) DO NOTHING;

-- ─── role → permission grants ───────────────────────────────────────
WITH grants (role_code, permission_code) AS (
    VALUES
        -- driver
        ('driver', 'profile.read'), ('driver', 'profile.update'),
        ('driver', 'drivers.read'), ('driver', 'drivers.update'),
        ('driver', 'vehicles.create'), ('driver', 'vehicles.update'), ('driver', 'vehicles.delete'),
        ('driver', 'companies.read'),
        ('driver', 'loads.read'),
        ('driver', 'bookings.create'), ('driver', 'bookings.respond'),
        ('driver', 'bookings.cancel'), ('driver', 'bookings.read'),
        ('driver', 'trips.read'), ('driver', 'trips.update'), ('driver', 'trips.track'),
        ('driver', 'chat.read'), ('driver', 'chat.write'),
        ('driver', 'payments.read'), ('driver', 'payments.manage'),
        ('driver', 'complaints.create'),

        -- dispatcher
        ('dispatcher', 'profile.read'), ('dispatcher', 'profile.update'),
        ('dispatcher', 'drivers.read'), ('dispatcher', 'drivers.search'),
        ('dispatcher', 'companies.read'),
        ('dispatcher', 'loads.read'), ('dispatcher', 'loads.create'),
        ('dispatcher', 'loads.update'), ('dispatcher', 'loads.publish'),
        ('dispatcher', 'bookings.create'), ('dispatcher', 'bookings.respond'),
        ('dispatcher', 'bookings.cancel'), ('dispatcher', 'bookings.read'),
        ('dispatcher', 'trips.read'), ('dispatcher', 'trips.track'), ('dispatcher', 'trips.update'),
        ('dispatcher', 'chat.read'), ('dispatcher', 'chat.write'),
        ('dispatcher', 'complaints.create'),

        -- company_manager (dispatcher + company and money control)
        ('company_manager', 'profile.read'), ('company_manager', 'profile.update'),
        ('company_manager', 'drivers.read'), ('company_manager', 'drivers.search'),
        ('company_manager', 'companies.read'), ('company_manager', 'companies.update'),
        ('company_manager', 'companies.members'),
        ('company_manager', 'loads.read'), ('company_manager', 'loads.create'),
        ('company_manager', 'loads.update'), ('company_manager', 'loads.delete'),
        ('company_manager', 'loads.publish'),
        ('company_manager', 'bookings.create'), ('company_manager', 'bookings.respond'),
        ('company_manager', 'bookings.cancel'), ('company_manager', 'bookings.read'),
        ('company_manager', 'trips.read'), ('company_manager', 'trips.track'),
        ('company_manager', 'trips.update'), ('company_manager', 'trips.cancel'),
        ('company_manager', 'chat.read'), ('company_manager', 'chat.write'),
        ('company_manager', 'payments.read'), ('company_manager', 'payments.manage'),
        ('company_manager', 'payments.escrow'),
        ('company_manager', 'complaints.create'),

        -- admin
        ('admin', 'profile.read'), ('admin', 'profile.update'),
        ('admin', 'drivers.read'), ('admin', 'drivers.verify'),
        ('admin', 'drivers.block'), ('admin', 'drivers.search'),
        ('admin', 'vehicles.verify'),
        ('admin', 'companies.read'), ('admin', 'companies.verify'), ('admin', 'companies.block'),
        ('admin', 'loads.read'), ('admin', 'loads.moderate'),
        ('admin', 'bookings.read'),
        ('admin', 'trips.read'), ('admin', 'trips.track'), ('admin', 'trips.cancel'),
        ('admin', 'chat.read'),
        ('admin', 'payments.read'), ('admin', 'payments.refund'),
        ('admin', 'complaints.manage'),
        ('admin', 'admin.dashboard'), ('admin', 'admin.users'), ('admin', 'admin.reports'),
        ('admin', 'admin.notifications'), ('admin', 'admin.audit'), ('admin', 'admin.cms')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM grants g
JOIN roles r ON r.code = g.role_code
JOIN permissions p ON p.code = g.permission_code
ON CONFLICT DO NOTHING;

-- super_admin gets everything, always.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r CROSS JOIN permissions p
WHERE r.code = 'super_admin'
ON CONFLICT DO NOTHING;

-- ─── geography ──────────────────────────────────────────────────────
INSERT INTO countries (code, code3, name_uz, name_ru, name_en, phone_code, currency_code, flag_emoji, sort_order) VALUES
    ('UZ', 'UZB', 'O''zbekiston', 'Узбекистан', 'Uzbekistan',  '+998', 'UZS', '🇺🇿', 1),
    ('KZ', 'KAZ', 'Qozog''iston', 'Казахстан',  'Kazakhstan',  '+7',   'KZT', '🇰🇿', 2),
    ('KG', 'KGZ', 'Qirg''iziston','Кыргызстан', 'Kyrgyzstan',  '+996', 'KGS', '🇰🇬', 3),
    ('TJ', 'TJK', 'Tojikiston',   'Таджикистан','Tajikistan',  '+992', 'TJS', '🇹🇯', 4),
    ('TM', 'TKM', 'Turkmaniston', 'Туркменистан','Turkmenistan','+993', 'TMT', '🇹🇲', 5),
    ('RU', 'RUS', 'Rossiya',      'Россия',     'Russia',      '+7',   'RUB', '🇷🇺', 6),
    ('AF', 'AFG', 'Afg''oniston', 'Афганистан', 'Afghanistan', '+93',  'AFN', '🇦🇫', 7),
    ('CN', 'CHN', 'Xitoy',        'Китай',      'China',       '+86',  'CNY', '🇨🇳', 8),
    ('TR', 'TUR', 'Turkiya',      'Турция',     'Turkey',      '+90',  'TRY', '🇹🇷', 9)
ON CONFLICT (code) DO NOTHING;

WITH uz AS (SELECT id FROM countries WHERE code = 'UZ'),
regions_data (code, name_uz, name_ru, name_en, lat, lng, sort_order) AS (
    VALUES
        ('TAS_CITY', 'Toshkent shahri',    'город Ташкент',        'Tashkent City',       41.2995, 69.2401,  1),
        ('TAS',      'Toshkent viloyati',  'Ташкентская область',  'Tashkent Region',     41.0000, 69.5000,  2),
        ('AND',      'Andijon',            'Андижанская область',  'Andijan Region',      40.7821, 72.3442,  3),
        ('BUX',      'Buxoro',             'Бухарская область',    'Bukhara Region',      39.7681, 64.4556,  4),
        ('FAR',      'Farg''ona',          'Ферганская область',   'Fergana Region',      40.3864, 71.7864,  5),
        ('JIZ',      'Jizzax',             'Джизакская область',   'Jizzakh Region',      40.1158, 67.8422,  6),
        ('QAS',      'Qashqadaryo',        'Кашкадарьинская область','Kashkadarya Region',38.8610, 65.7890,  7),
        ('NAV',      'Navoiy',             'Навоийская область',   'Navoiy Region',       40.1033, 65.3733,  8),
        ('NAM',      'Namangan',           'Наманганская область', 'Namangan Region',     40.9983, 71.6726,  9),
        ('SAM',      'Samarqand',          'Самаркандская область','Samarkand Region',    39.6270, 66.9750, 10),
        ('SUR',      'Surxondaryo',        'Сурхандарьинская область','Surkhandarya Region',37.2242,67.2783,11),
        ('SIR',      'Sirdaryo',           'Сырдарьинская область','Sirdaryo Region',     40.3833, 68.7167, 12),
        ('XOR',      'Xorazm',             'Хорезмская область',   'Khorezm Region',      41.5500, 60.6333, 13),
        ('QOR',      'Qoraqalpog''iston',  'Каракалпакстан',       'Karakalpakstan',      42.4600, 59.6100, 14)
)
INSERT INTO regions (country_id, code, name_uz, name_ru, name_en, center, sort_order)
SELECT uz.id, d.code, d.name_uz, d.name_ru, d.name_en,
       ST_SetSRID(ST_MakePoint(d.lng, d.lat), 4326)::geography, d.sort_order
FROM regions_data d CROSS JOIN uz
ON CONFLICT (country_id, code) DO NOTHING;

WITH cities_data (region_code, name_uz, name_ru, name_en, lat, lng, is_capital) AS (
    VALUES
        ('TAS_CITY', 'Toshkent',   'Ташкент',    'Tashkent',   41.2995, 69.2401, TRUE),
        ('TAS',      'Chirchiq',   'Чирчик',     'Chirchiq',   41.4689, 69.5822, FALSE),
        ('TAS',      'Angren',     'Ангрен',     'Angren',     41.0167, 70.1435, FALSE),
        ('TAS',      'Olmaliq',    'Алмалык',    'Olmaliq',    40.8447, 69.5982, FALSE),
        ('AND',      'Andijon',    'Андижан',    'Andijan',    40.7821, 72.3442, FALSE),
        ('BUX',      'Buxoro',     'Бухара',     'Bukhara',    39.7681, 64.4556, FALSE),
        ('BUX',      'Kogon',      'Каган',      'Kogon',      39.7222, 64.5528, FALSE),
        ('FAR',      'Farg''ona',  'Фергана',    'Fergana',    40.3864, 71.7864, FALSE),
        ('FAR',      'Qo''qon',    'Коканд',     'Kokand',     40.5286, 70.9425, FALSE),
        ('FAR',      'Marg''ilon', 'Маргилан',   'Margilan',   40.4711, 71.7242, FALSE),
        ('JIZ',      'Jizzax',     'Джизак',     'Jizzakh',    40.1158, 67.8422, FALSE),
        ('QAS',      'Qarshi',     'Карши',      'Karshi',     38.8606, 65.7891, FALSE),
        ('QAS',      'Shahrisabz', 'Шахрисабз',  'Shakhrisabz',39.0578, 66.8306, FALSE),
        ('NAV',      'Navoiy',     'Навои',      'Navoiy',     40.1033, 65.3733, FALSE),
        ('NAV',      'Zarafshon',  'Зарафшан',   'Zarafshan',  41.5772, 64.2050, FALSE),
        ('NAM',      'Namangan',   'Наманган',   'Namangan',   40.9983, 71.6726, FALSE),
        ('SAM',      'Samarqand',  'Самарканд',  'Samarkand',  39.6270, 66.9750, FALSE),
        ('SAM',      'Kattaqo''rg''on','Каттакурган','Kattakurgan',39.8992,66.2606,FALSE),
        ('SUR',      'Termiz',     'Термез',     'Termez',     37.2242, 67.2783, FALSE),
        ('SUR',      'Denov',      'Денау',      'Denau',      38.2678, 67.8944, FALSE),
        ('SIR',      'Guliston',   'Гулистан',   'Gulistan',   40.4897, 68.7842, FALSE),
        ('XOR',      'Urganch',    'Ургенч',     'Urgench',    41.5500, 60.6333, FALSE),
        ('XOR',      'Xiva',       'Хива',       'Khiva',      41.3783, 60.3639, FALSE),
        ('QOR',      'Nukus',      'Нукус',      'Nukus',      42.4600, 59.6100, FALSE),
        ('QOR',      'Beruniy',    'Беруни',     'Beruniy',    41.6931, 60.7522, FALSE)
)
INSERT INTO cities (region_id, name_uz, name_ru, name_en, location, is_capital)
SELECT r.id, d.name_uz, d.name_ru, d.name_en,
       ST_SetSRID(ST_MakePoint(d.lng, d.lat), 4326)::geography, d.is_capital
FROM cities_data d
JOIN countries c ON c.code = 'UZ'
JOIN regions r ON r.code = d.region_code AND r.country_id = c.id
WHERE NOT EXISTS (
    SELECT 1 FROM cities x WHERE x.region_id = r.id AND x.name_en = d.name_en
);

-- ─── cargo types ────────────────────────────────────────────────────
INSERT INTO cargo_types (code, name_uz, name_ru, name_en, icon, requires_adr, requires_temperature, sort_order) VALUES
    ('general',       'Umumiy yuk',        'Генеральный груз',   'General cargo',      'package',    FALSE, FALSE,  1),
    ('food',          'Oziq-ovqat',        'Продукты питания',   'Food products',      'utensils',   FALSE, FALSE,  2),
    ('perishable',    'Tez buziluvchi',    'Скоропортящийся',    'Perishable',         'snowflake',  FALSE, TRUE,   3),
    ('construction',  'Qurilish mollari',  'Стройматериалы',     'Construction',       'hard-hat',   FALSE, FALSE,  4),
    ('agriculture',   'Qishloq xo''jaligi','Сельхозпродукция',   'Agricultural',       'wheat',      FALSE, FALSE,  5),
    ('textile',       'To''qimachilik',    'Текстиль',           'Textile',            'shirt',      FALSE, FALSE,  6),
    ('furniture',     'Mebel',             'Мебель',             'Furniture',          'armchair',   FALSE, FALSE,  7),
    ('equipment',     'Uskunalar',         'Оборудование',       'Equipment',          'cog',        FALSE, FALSE,  8),
    ('vehicles',      'Avtotransport',     'Автотранспорт',      'Vehicles',           'car',        FALSE, FALSE,  9),
    ('chemicals',     'Kimyoviy moddalar', 'Химические вещества','Chemicals',          'flask',      TRUE,  FALSE, 10),
    ('fuel',          'Yoqilg''i',         'Топливо',            'Fuel',               'fuel',       TRUE,  FALSE, 11),
    ('metal',         'Metall',            'Металл',             'Metal',              'anvil',      FALSE, FALSE, 12),
    ('grain',         'Don mahsulotlari',  'Зерновые',           'Grain',              'wheat',      FALSE, FALSE, 13),
    ('livestock',     'Chorva mollari',    'Скот',               'Livestock',          'cow',        FALSE, FALSE, 14),
    ('medical',       'Tibbiy mahsulotlar','Медицинские товары', 'Medical supplies',   'pill',       FALSE, TRUE,  15),
    ('oversized',     'Yirik gabaritli',   'Негабаритный груз',  'Oversized',          'expand',     FALSE, FALSE, 16),
    ('other',         'Boshqa',            'Другое',             'Other',              'box',        FALSE, FALSE, 99)
ON CONFLICT (code) DO NOTHING;

-- ─── subscription plans ─────────────────────────────────────────────
INSERT INTO subscription_plans (code, name_uz, name_ru, name_en, target_role, price_amount, currency, period, trial_days, features, limits, is_default, sort_order) VALUES
    ('driver_basic', 'Bazaviy', 'Базовый', 'Basic', 'driver', 0, 'UZS', 'month', 0,
     '["load_search","5_contact_reveals_per_day","chat","trip_tracking"]'::JSONB,
     '{"contact_reveals_per_day":5,"saved_searches":3,"vehicles":1}'::JSONB, TRUE, 1),
    ('driver_premium_m', 'Premium (oylik)', 'Премиум (месяц)', 'Premium (monthly)', 'driver', 4900000, 'UZS', 'month', 7,
     '["unlimited_contact_reveals","priority_matching","early_load_access","advanced_filters","no_ads"]'::JSONB,
     '{"contact_reveals_per_day":-1,"saved_searches":20,"vehicles":5}'::JSONB, FALSE, 2),
    ('driver_premium_y', 'Premium (yillik)', 'Премиум (год)', 'Premium (yearly)', 'driver', 49000000, 'UZS', 'year', 7,
     '["unlimited_contact_reveals","priority_matching","early_load_access","advanced_filters","no_ads","2_months_free"]'::JSONB,
     '{"contact_reveals_per_day":-1,"saved_searches":20,"vehicles":5}'::JSONB, FALSE, 3),

    ('company_start', 'Start', 'Старт', 'Start', 'company', 0, 'UZS', 'month', 0,
     '["3_active_loads","driver_search","chat","basic_analytics"]'::JSONB,
     '{"active_loads":3,"dispatchers":1,"driver_map":false}'::JSONB, TRUE, 1),
    ('company_business_m', 'Business (oylik)', 'Бизнес (месяц)', 'Business (monthly)', 'company', 29900000, 'UZS', 'month', 14,
     '["50_active_loads","live_driver_map","featured_loads","advanced_analytics","5_dispatchers","priority_support"]'::JSONB,
     '{"active_loads":50,"dispatchers":5,"driver_map":true,"featured_loads_per_month":10}'::JSONB, FALSE, 2),
    ('company_business_y', 'Business (yillik)', 'Бизнес (год)', 'Business (yearly)', 'company', 299000000, 'UZS', 'year', 14,
     '["50_active_loads","live_driver_map","featured_loads","advanced_analytics","5_dispatchers","priority_support","2_months_free"]'::JSONB,
     '{"active_loads":50,"dispatchers":5,"driver_map":true,"featured_loads_per_month":10}'::JSONB, FALSE, 3),
    ('company_enterprise_m', 'Enterprise', 'Энтерпрайз', 'Enterprise', 'company', 99900000, 'UZS', 'month', 0,
     '["unlimited_loads","live_driver_map","featured_loads","full_analytics","unlimited_dispatchers","api_access","dedicated_manager"]'::JSONB,
     '{"active_loads":-1,"dispatchers":-1,"driver_map":true,"featured_loads_per_month":-1,"api_access":true}'::JSONB, FALSE, 4)
ON CONFLICT (code) DO NOTHING;

-- ─── platform wallet ────────────────────────────────────────────────
INSERT INTO wallets (owner_type, owner_id, currency, balance)
SELECT 'platform', NULL, 'UZS', 0
WHERE NOT EXISTS (SELECT 1 FROM wallets WHERE owner_type = 'platform' AND currency = 'UZS');

-- ─── system settings ────────────────────────────────────────────────
INSERT INTO system_settings (key, value, value_type, group_name, description, is_public) VALUES
    ('platform.commission_percent',   '5'::JSONB,       'number',  'payments', 'Platform commission on released escrow', TRUE),
    ('platform.min_load_price',       '50000'::JSONB,   'number',  'loads',    'Minimum load price in UZS tiyin', TRUE),
    ('loads.default_expiry_hours',    '72'::JSONB,      'number',  'loads',    'Hours a published load stays live', TRUE),
    ('loads.max_photos',              '10'::JSONB,      'number',  'loads',    'Maximum photos per load', TRUE),
    ('bookings.offer_ttl_minutes',    '30'::JSONB,      'number',  'bookings', 'Minutes a company offer stays open', TRUE),
    ('bookings.reservation_ttl_hours','24'::JSONB,      'number',  'bookings', 'Hours a driver reservation stays pending', TRUE),
    ('tracking.interval_trip_sec',    '15'::JSONB,      'number',  'tracking', 'Location report interval during a trip', TRUE),
    ('tracking.interval_idle_sec',    '60'::JSONB,      'number',  'tracking', 'Location report interval while idle-online', TRUE),
    ('tracking.retention_days',       '90'::JSONB,      'number',  'tracking', 'Days of hot location history', FALSE),
    ('support.phone',                 '"+998712000000"'::JSONB, 'string', 'support', 'Support hotline', TRUE),
    ('support.telegram',              '"@loadify_support"'::JSONB, 'string', 'support', 'Support Telegram', TRUE),
    ('trips.auto_confirm_hours',      '72'::JSONB,      'number',  'trips',    'Auto-confirm delivery after this many hours', TRUE),
    ('security.contact_reveal_free',  '5'::JSONB,       'number',  'security', 'Free contact reveals per day', TRUE)
ON CONFLICT (key) DO NOTHING;

-- ─── feature flags ──────────────────────────────────────────────────
INSERT INTO feature_flags (key, description, is_enabled, rollout_percent) VALUES
    ('payments.escrow',        'Escrow payments',            FALSE, 0),
    ('payments.click',         'Click checkout',             FALSE, 0),
    ('payments.payme',         'Payme checkout',             FALSE, 0),
    ('payments.uzum',          'Uzum Bank checkout',         FALSE, 0),
    ('ai.load_matching',       'AI load recommendations',    FALSE, 0),
    ('ai.price_prediction',    'AI price suggestions',       FALSE, 0),
    ('ai.fraud_detection',     'Automated fraud scoring',    TRUE,  100),
    ('international.enabled',  'Cross-border loads',         FALSE, 0)
ON CONFLICT (key) DO NOTHING;
