-- Launch languages (proposed list in spec F3, still to be confirmed).
-- Adding a language is a data change: add a row here, sources in
-- infra/youtube_sources.yaml and level keywords in app_config.
INSERT INTO languages (code, english_name, native_name, script, direction, segmenter, is_learnable, is_native_option, is_ui, launch_order) VALUES
  ('en',      'English',                        'English',          'Latin',      'ltr', 'space',  true, true, true,  1),
  ('es',      'Spanish',                        'Español',          'Latin',      'ltr', 'space',  true, true, true,  2),
  ('fr',      'French',                         'Français',         'Latin',      'ltr', 'space',  true, true, true,  3),
  ('ja',      'Japanese',                       '日本語',            'Japanese',   'ltr', 'kagome', true, true, true,  4),
  ('de',      'German',                         'Deutsch',          'Latin',      'ltr', 'space',  true, true, true,  5),
  ('ko',      'Korean',                         '한국어',            'Hangul',     'ltr', 'space',  true, true, true,  6),
  ('it',      'Italian',                        'Italiano',         'Latin',      'ltr', 'space',  true, true, false, 7),
  ('hi',      'Hindi',                          'हिन्दी',             'Devanagari', 'ltr', 'space',  true, true, true,  8),
  ('zh-Hans', 'Chinese (Mandarin, Simplified)', '中文（简体）',       'Han',        'ltr', 'gse',    true, true, true,  9),
  ('ru',      'Russian',                        'Русский',          'Cyrillic',   'ltr', 'space',  true, true, false, 10),
  ('ar',      'Arabic',                         'العربية',          'Arabic',     'rtl', 'space',  true, true, true,  11),
  ('pt-BR',   'Portuguese (Brazil)',            'Português (Brasil)', 'Latin',    'ltr', 'space',  true, true, true,  12),
  ('tr',      'Turkish',                        'Türkçe',           'Latin',      'ltr', 'space',  true, true, true,  13),
  ('nl',      'Dutch',                          'Nederlands',       'Latin',      'ltr', 'space',  true, true, false, 14),
  ('sv',      'Swedish',                        'Svenska',          'Latin',      'ltr', 'space',  true, true, false, 15)
ON CONFLICT (code) DO UPDATE SET
  english_name = EXCLUDED.english_name, native_name = EXCLUDED.native_name,
  script = EXCLUDED.script, direction = EXCLUDED.direction, segmenter = EXCLUDED.segmenter,
  is_learnable = EXCLUDED.is_learnable, is_native_option = EXCLUDED.is_native_option,
  is_ui = EXCLUDED.is_ui, launch_order = EXCLUDED.launch_order;
