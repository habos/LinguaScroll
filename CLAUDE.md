# LinguaScroll

iOS (SwiftUI) swipe feed of YouTube Shorts for language learners, with a Go backend. The current target is v0: YouTube-only, anonymous device IDs, no accounts, uploads or ads.

- Read the matching skill in `.claude/skills/` before working on an area: `linguascroll-backend`, `linguascroll-ios`, `linguascroll-ux`, `linguascroll-qa`, `linguascroll-feed-ranking`, `linguascroll-youtube-rules`.
- The spec is split into modules V1 to V7 ("v0 build" tab of the spec doc). Work on one module per change.
- `api/openapi.yaml` is the contract. Change it first, then `make gen`; never hand-edit `backend/internal/api/api.gen.go` or the Swift spec copy.
- Before pushing: `make lint test` (tests need Postgres at `TEST_DATABASE_URL`; `make db` starts one).
- Never put anything over the YouTube player, and never call the real YouTube API from tests.
