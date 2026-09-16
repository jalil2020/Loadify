-- 000013 · Geography beyond Uzbekistan: the corridors adverts actually name
--
-- The marketplace search only ever needed the fourteen Uzbek regions. An advert
-- posted to a Telegram group names Moscow, Almaty or Urumqi in the same breath,
-- and a name the parser cannot resolve is a name it has to report as not
-- understood, so the vocabulary has to reach as far as the trucks do.
--
-- Coordinates are deliberately left NULL. These rows exist to resolve names;
-- the distance search works off each load's own point, and inventing a
-- coordinate to fill the column would put a pin somewhere nobody chose.
--
-- Idempotent: safe to re-run.

WITH regions_data (country_code, code, name_uz, name_ru, name_en) AS (VALUES
    -- Russia: the destination on most of these corridors.
    ('RU', 'RU-MOW', 'Moskva shahri',            'город Москва',                 'Moscow City'),
    ('RU', 'RU-MOS', 'Moskva viloyati',          'Московская область',           'Moscow Oblast'),
    ('RU', 'RU-SPE', 'Sankt-Peterburg',          'Санкт-Петербург',              'Saint Petersburg'),
    ('RU', 'RU-LEN', 'Leningrad viloyati',       'Ленинградская область',        'Leningrad Oblast'),
    ('RU', 'RU-NVS', 'Novosibirsk viloyati',     'Новосибирская область',        'Novosibirsk Oblast'),
    ('RU', 'RU-SVE', 'Sverdlovsk viloyati',      'Свердловская область',         'Sverdlovsk Oblast'),
    ('RU', 'RU-TA',  'Tatariston',               'Татарстан',                    'Tatarstan'),
    ('RU', 'RU-SAM', 'Samara viloyati',          'Самарская область',            'Samara Oblast'),
    ('RU', 'RU-ROS', 'Rostov viloyati',          'Ростовская область',           'Rostov Oblast'),
    ('RU', 'RU-KDA', 'Krasnodar olkasi',         'Краснодарский край',           'Krasnodar Krai'),
    ('RU', 'RU-CHE', 'Chelyabinsk viloyati',     'Челябинская область',          'Chelyabinsk Oblast'),
    ('RU', 'RU-BA',  'Boshqirdiston',            'Башкортостан',                 'Bashkortostan'),
    ('RU', 'RU-NIZ', 'Nijniy Novgorod viloyati', 'Нижегородская область',        'Nizhny Novgorod Oblast'),
    ('RU', 'RU-VGG', 'Volgograd viloyati',       'Волгоградская область',        'Volgograd Oblast'),
    ('RU', 'RU-VOR', 'Voronej viloyati',         'Воронежская область',          'Voronezh Oblast'),
    ('RU', 'RU-OMS', 'Omsk viloyati',            'Омская область',               'Omsk Oblast'),
    ('RU', 'RU-TYU', 'Tyumen viloyati',          'Тюменская область',            'Tyumen Oblast'),
    ('RU', 'RU-KYA', 'Krasnoyarsk olkasi',       'Красноярский край',            'Krasnoyarsk Krai'),
    ('RU', 'RU-PER', 'Perm olkasi',              'Пермский край',                'Perm Krai'),
    ('RU', 'RU-SAR', 'Saratov viloyati',         'Саратовская область',          'Saratov Oblast'),
    ('RU', 'RU-ORE', 'Orenburg viloyati',        'Оренбургская область',         'Orenburg Oblast'),

    -- Kazakhstan: both a destination and the transit country for everything
    -- heading north.
    ('KZ', 'KZ-ALA', 'Almati shahri',            'город Алматы',                 'Almaty City'),
    ('KZ', 'KZ-AST', 'Ostona shahri',            'город Астана',                 'Astana City'),
    ('KZ', 'KZ-SHY', 'Shimkent shahri',          'город Шымкент',                'Shymkent City'),
    ('KZ', 'KZ-AKT', 'Aqtobe viloyati',          'Актюбинская область',          'Aktobe Region'),
    ('KZ', 'KZ-KAR', 'Qaraganda viloyati',       'Карагандинская область',       'Karaganda Region'),
    ('KZ', 'KZ-ATY', 'Atirau viloyati',          'Атырауская область',           'Atyrau Region'),
    ('KZ', 'KZ-MAN', 'Mangistau viloyati',       'Мангистауская область',        'Mangystau Region'),
    ('KZ', 'KZ-KZY', 'Qizilorda viloyati',       'Кызылординская область',       'Kyzylorda Region'),
    ('KZ', 'KZ-ZHA', 'Jambil viloyati',          'Жамбылская область',           'Zhambyl Region'),
    ('KZ', 'KZ-PAV', 'Pavlodar viloyati',        'Павлодарская область',         'Pavlodar Region'),
    ('KZ', 'KZ-VOS', 'Sharqiy Qozogiston',       'Восточно-Казахстанская область','East Kazakhstan Region'),

    ('KG', 'KG-BIS', 'Bishkek shahri',           'город Бишкек',                 'Bishkek City'),
    ('KG', 'KG-OSH', 'Osh viloyati',             'Ошская область',               'Osh Region'),

    ('TJ', 'TJ-DU',  'Dushanbe shahri',          'город Душанбе',                'Dushanbe City'),
    ('TJ', 'TJ-SU',  'Sugd viloyati',            'Согдийская область',           'Sughd Region'),

    ('TM', 'TM-A',   'Ashxabad shahri',          'город Ашхабад',                'Ashgabat City'),
    ('TM', 'TM-L',   'Lebap velayati',           'Лебапский велаят',             'Lebap Province'),

    ('AF', 'AF-KAB', 'Kobul viloyati',           'Провинция Кабул',              'Kabul Province'),
    ('AF', 'AF-BAL', 'Balx viloyati',            'Провинция Балх',               'Balkh Province'),

    ('TR', 'TR-34',  'Istanbul',                 'Стамбул',                      'Istanbul'),
    ('TR', 'TR-06',  'Anqara',                   'Анкара',                       'Ankara'),
    ('TR', 'TR-33',  'Mersin',                   'Мерсин',                       'Mersin'),

    ('CN', 'CN-XJ',  'Shinjon',                  'Синьцзян',                     'Xinjiang'),
    ('CN', 'CN-ZJ',  'Chjetszyan',               'Чжэцзян',                      'Zhejiang'),
    ('CN', 'CN-GD',  'Guandun',                  'Гуандун',                      'Guangdong')
)
INSERT INTO regions (country_id, code, name_uz, name_ru, name_en, sort_order)
SELECT c.id, d.code, d.name_uz, d.name_ru, d.name_en, 50
FROM regions_data d
JOIN countries c ON c.code = d.country_code
ON CONFLICT (country_id, code) DO NOTHING;

WITH cities_data (country_code, region_code, name_uz, name_ru, name_en, is_capital) AS (VALUES
    ('RU', 'RU-MOW', 'Moskva',           'Москва',            'Moscow',            TRUE),
    ('RU', 'RU-SPE', 'Sankt-Peterburg',  'Санкт-Петербург',   'Saint Petersburg',  FALSE),
    ('RU', 'RU-NVS', 'Novosibirsk',      'Новосибирск',       'Novosibirsk',       FALSE),
    ('RU', 'RU-SVE', 'Yekaterinburg',    'Екатеринбург',      'Yekaterinburg',     FALSE),
    ('RU', 'RU-TA',  'Qozon',            'Казань',            'Kazan',             FALSE),
    ('RU', 'RU-SAM', 'Samara',           'Самара',            'Samara',            FALSE),
    ('RU', 'RU-ROS', 'Rostov-na-Donu',   'Ростов-на-Дону',    'Rostov-on-Don',     FALSE),
    ('RU', 'RU-KDA', 'Krasnodar',        'Краснодар',         'Krasnodar',         FALSE),
    ('RU', 'RU-CHE', 'Chelyabinsk',      'Челябинск',         'Chelyabinsk',       FALSE),
    ('RU', 'RU-BA',  'Ufa',              'Уфа',               'Ufa',               FALSE),
    ('RU', 'RU-NIZ', 'Nijniy Novgorod',  'Нижний Новгород',   'Nizhny Novgorod',   FALSE),
    ('RU', 'RU-VGG', 'Volgograd',        'Волгоград',         'Volgograd',         FALSE),
    ('RU', 'RU-VOR', 'Voronej',          'Воронеж',           'Voronezh',          FALSE),
    ('RU', 'RU-OMS', 'Omsk',             'Омск',              'Omsk',              FALSE),
    ('RU', 'RU-TYU', 'Tyumen',           'Тюмень',            'Tyumen',            FALSE),
    ('RU', 'RU-KYA', 'Krasnoyarsk',      'Красноярск',        'Krasnoyarsk',       FALSE),
    ('RU', 'RU-PER', 'Perm',             'Пермь',             'Perm',              FALSE),
    ('RU', 'RU-SAR', 'Saratov',          'Саратов',           'Saratov',           FALSE),
    ('RU', 'RU-ORE', 'Orenburg',         'Оренбург',          'Orenburg',          FALSE),

    ('KZ', 'KZ-ALA', 'Almati',           'Алматы',            'Almaty',            FALSE),
    ('KZ', 'KZ-AST', 'Ostona',           'Астана',            'Astana',            TRUE),
    ('KZ', 'KZ-SHY', 'Shimkent',         'Шымкент',           'Shymkent',          FALSE),
    ('KZ', 'KZ-AKT', 'Aqtobe',           'Актобе',            'Aktobe',            FALSE),
    ('KZ', 'KZ-KAR', 'Qaraganda',        'Караганда',         'Karaganda',         FALSE),
    ('KZ', 'KZ-ATY', 'Atirau',           'Атырау',            'Atyrau',            FALSE),
    ('KZ', 'KZ-MAN', 'Aqtov',            'Актау',             'Aktau',             FALSE),
    ('KZ', 'KZ-KZY', 'Qizilorda',        'Кызылорда',         'Kyzylorda',         FALSE),
    ('KZ', 'KZ-ZHA', 'Taroz',            'Тараз',             'Taraz',             FALSE),
    ('KZ', 'KZ-PAV', 'Pavlodar',         'Павлодар',          'Pavlodar',          FALSE),
    ('KZ', 'KZ-VOS', 'Oskemen',          'Усть-Каменогорск',  'Oskemen',           FALSE),

    ('KG', 'KG-BIS', 'Bishkek',          'Бишкек',            'Bishkek',           TRUE),
    ('KG', 'KG-OSH', 'Osh',              'Ош',                'Osh',               FALSE),

    ('TJ', 'TJ-DU',  'Dushanbe',         'Душанбе',           'Dushanbe',          TRUE),
    ('TJ', 'TJ-SU',  'Xujand',           'Худжанд',           'Khujand',           FALSE),

    ('TM', 'TM-A',   'Ashxabad',         'Ашхабад',           'Ashgabat',          TRUE),
    ('TM', 'TM-L',   'Turkmanobod',      'Туркменабад',       'Turkmenabat',       FALSE),

    ('AF', 'AF-KAB', 'Kobul',            'Кабул',             'Kabul',             TRUE),
    ('AF', 'AF-BAL', 'Mazori Sharif',    'Мазари-Шариф',      'Mazar-i-Sharif',    FALSE),
    ('AF', 'AF-BAL', 'Hayraton',         'Хайратон',          'Hairatan',          FALSE),

    ('TR', 'TR-34',  'Istanbul',         'Стамбул',           'Istanbul',          FALSE),
    ('TR', 'TR-06',  'Anqara',           'Анкара',            'Ankara',            TRUE),
    ('TR', 'TR-33',  'Mersin',           'Мерсин',            'Mersin',            FALSE),

    ('CN', 'CN-XJ',  'Urumchi',          'Урумчи',            'Urumqi',            FALSE),
    ('CN', 'CN-XJ',  'Qashqar',          'Кашгар',            'Kashgar',           FALSE),
    ('CN', 'CN-ZJ',  'Iyu',              'Иу',                'Yiwu',              FALSE),
    ('CN', 'CN-GD',  'Guanchjou',        'Гуанчжоу',          'Guangzhou',         FALSE)
)
INSERT INTO cities (region_id, name_uz, name_ru, name_en, is_capital)
SELECT r.id, d.name_uz, d.name_ru, d.name_en, d.is_capital
FROM cities_data d
JOIN countries c ON c.code = d.country_code
JOIN regions r ON r.code = d.region_code AND r.country_id = c.id
WHERE NOT EXISTS (
    SELECT 1 FROM cities x WHERE x.region_id = r.id AND x.name_en = d.name_en
);
