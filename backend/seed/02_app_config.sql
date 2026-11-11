-- Runtime config defaults (v0 shared contracts). Tuning changes go here or
-- straight into the table, never into Go constants.
INSERT INTO app_config (key, value) VALUES
  ('feed.page_size',                  '10'),
  ('feed.level_window',               '1'),
  ('feed.explore_share',              '0.15'),
  ('feed.channel_spacing',            '5'),
  ('feed.music_cap',                  '0.2'),
  ('feed.explore_max_views',          '50'),
  ('feed.candidates',                 '400'),
  ('feed.weights',                    '{"completion": 0.25, "views_pct": 0.15, "likes_pct": 0.15, "level": 0.10, "channel": 0.15, "category": 0.10, "fresh": 0.05, "hide": -0.30, "noise": 0.05}'),
  ('youtube.daily_quota_units',       '9000'),
  ('youtube.min_language_confidence', '0.8'),
  ('youtube.level_keywords',          '{"1": ["beginner", "beginners", "a1", "slow", "easy", "principiante", "principiantes", "débutant", "débutants", "anfänger", "初心者", "초급", "principianti", "beginners"], "3": ["intermediate", "b1", "b2", "intermedio", "intermédiaire", "mittelstufe", "中級", "중급"], "5": ["advanced", "c1", "c2", "native speed", "avanzado", "avancé", "fortgeschritten", "上級", "고급"]}'),
  ('youtube.captions_default',        'true'),
  ('reports.hide_after',              '3')
ON CONFLICT (key) DO NOTHING;
