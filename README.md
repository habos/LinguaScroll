# LinguaScroll

A vertical swipe feed of short videos in the language you're learning, so learning feels like watching TV. iOS first (SwiftUI), Go backend.

**Current target: v0**, a YouTube-only build: the feed is YouTube Shorts found with the YouTube Data API, ranked from metadata and how each device watches. No accounts (each install gets a random ID in the Keychain), no uploads, no ads.

- Spec: see the "v0 build" tab of the product spec doc (modules V1 to V7).
- Project skills for Claude live in `.claude/skills/` (UX, iOS, backend, QA, feed ranking, YouTube rules).

## Layout (planned)

```
api/            OpenAPI contract (source of truth for Go server + Swift client)
backend/        Go API, worker, migrations
ios/            Xcode project and Swift packages
infra/          docker-compose, YouTube source lists, deploy
```
