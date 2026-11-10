-- A few starter search terms per language. V3 replaces this with
-- infra/youtube_sources.yaml loaded by `make sources`; seed channels go there.
INSERT INTO youtube_sources (language, kind, value, level_hint) VALUES
  ('es', 'query', 'aprender español shorts', NULL),
  ('es', 'query', 'español para principiantes', 1),
  ('es', 'query', 'historias cortas en español', NULL),
  ('fr', 'query', 'apprendre le français shorts', NULL),
  ('fr', 'query', 'français facile', 1),
  ('fr', 'query', 'vie quotidienne en France', NULL),
  ('ja', 'query', '日本語 ショート', NULL),
  ('ja', 'query', 'やさしい日本語', 1),
  ('de', 'query', 'Deutsch lernen shorts', NULL),
  ('ko', 'query', '한국어 쇼츠', NULL),
  ('it', 'query', 'imparare italiano shorts', NULL),
  ('pt-BR', 'query', 'português brasileiro shorts', NULL),
  ('en', 'query', 'everyday english shorts', NULL)
ON CONFLICT (language, kind, value) DO NOTHING;
