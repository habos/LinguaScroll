# QA for LinguaScroll v0

This folder tracks whether each v0 module (V1 to V7 in the spec's "v0 build" tab) meets its acceptance criteria, and holds the checks that need a person or a real device.

Read `.claude/skills/linguascroll-qa` and `.claude/skills/linguascroll-youtube-rules` first.

## Files

| File | What it is |
|---|---|
| [`acceptance-matrix.md`](acceptance-matrix.md) | Every acceptance criterion for V1 to V7, plus extra cases QA added, with the test or manual check that covers it and its status |
| [`youtube-compliance.md`](youtube-compliance.md) | Release-blocking YouTube checks and how to verify each |
| [`manual/`](manual/) | Step-by-step scripts for checks a test can't do (real device, reinstall, feed quality) |
| [`sql/compliance.sql`](sql/compliance.sql) | Queries that must all return 0 rows against a running database |
| [`runs/`](runs/) | One file per QA pass: date, commit, what was run, pass/fail with output |

## Definition of done for a module

A module PR is done when:

1. Every row for that module in the matrix has a test name (automated) or a manual script (manual), and the PR description lists them.
2. Automated rows pass in CI (`make gen-check lint test`, Swift build/tests).
3. Manual rows have a run logged in `runs/` with the commit SHA.
4. If the change touches YouTube (Data API, stored metadata, player, anything near a clip), the YouTube review checklist in the PR template is ticked.

## Statuses used in the matrix

- `todo`: module not built yet
- `covered`: a test or script exists but hasn't been run against the current commit
- `pass` / `fail`: result of the latest run, with a link to the run log or CI job
- `gap`: criterion with no test and no script yet (QA owes one)

## Conventions

- Go: table-driven tests against a real Postgres (`dbtest.New`), never the real YouTube API (`FAKE_VENDORS=1`).
- Name a test after its matrix ID where possible, e.g. `TestV2_AgeBlocked` for V2-01, so `grep V2-01` finds it.
- A bug found in QA gets a failing test first, then the fix.
