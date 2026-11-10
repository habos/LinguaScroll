## What this changes

<!-- Module (V1 to V7) and a short summary. -->

## Acceptance criteria covered

<!-- List the qa/acceptance-matrix.md rows this PR covers and the test name or manual script for each, e.g.
- V2-01 birth year 2016 → 403, no row: `TestV2_AgeBlocked`
-->

## Testing

- [ ] `make gen-check lint test` passes
- [ ] Manual rows run and logged in `qa/runs/` (or say which are still owed)

## YouTube review (tick or write "n/a: doesn't touch YouTube")

- [ ] No view, gesture or overlay inside the player frame
- [ ] Autoplay / > 50% visible / single-audio rules intact
- [ ] Referer / `baseURL` unchanged
- [ ] New stored YouTube fields covered by the 25-day refresh and cleared on removal
- [ ] Quota cost estimated and within the daily budget
- [ ] No ads or paywall near YouTube content
- [ ] No real YouTube API calls from tests or CI
