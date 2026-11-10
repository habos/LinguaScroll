---
name: linguascroll-ux
description: UX and product design rules for LinguaScroll, the iOS swipe feed of YouTube Shorts for language learners. Use when designing or reviewing any screen, flow, copy or interaction in the app.
---

# LinguaScroll UX

LinguaScroll should feel like doomscrolling a TV show in the language you're learning. Learning is passive and never feels like study. Every design decision is judged against that.

## Current scope (v0, YouTube-only)

- Onboarding: birth year (13+ check), language to learn, native language, rough level. No account.
- Feed: vertical swipe of YouTube Shorts in the active learning language, played in the official YouTube player.
- Bar beside the player: like, save, share, more (hide, not interested in this channel, report), channel name.
- Me tab: Saved and Liked grids, Languages (add, level, remove, set active), Settings (native language, YouTube captions on/off, hidden channels, privacy note, Reset my data).
- Not in v0: sign-in, uploads, creators, profiles, comments, ads, tap-to-translate, My words, website. Don't design them in unless asked; if useful, note "later (v1/v2/v3)".

## Hard constraints from YouTube (never break)

- Nothing drawn over the YouTube player: no buttons, gradients, captions, badges, tooltips, coach marks.
- No tap, double-tap or long-press actions on the player. Like only through the bar.
- Player at least 200x200 pt. Autoplay only when the cell is more than half visible. Only one player playing.
- So the layout is: player fills the width at the clip's aspect from the top safe area; a bar of at least 64 pt under it holds every control.

## Principles

1. Zero friction to the first clip. Onboarding in under 30 seconds of taps, four questions, no sign-up wall, no permission prompts (no ATT, no notifications in v0).
2. The feed is the app. Two tabs only (Feed, Me). The active language sits in the top bar; tapping it switches language or opens Add language.
3. Calm, not gamified. No streaks, XP, nags or red badges. Immersion over drills.
4. Honest about the device ID. Privacy note says plainly: no account, a random ID on this phone, what is stored, how to reset.
5. Optimistic actions. Like and save change at once and roll back with a toast on failure. Hide and channel block animate the cell away and advance.
6. Errors don't stop the scroll. A broken clip auto-advances after 1 s; the user never sees a dead end.
7. Under-13 block is calm and final: "LinguaScroll is for ages 13 and up", no hint of the cut-off on the birth-year wheel, persists across reinstalls.

## Levels

- Internal scale 1 to 6 = CEFR A1 to C2. Show CEFR labels in the Languages screen.
- Onboarding wording: "just starting" (1), "can get by" (3), "pretty comfortable" (5).
- Level is rough in v0; don't design UI that implies precise grading.

## Copy and localisation

- Plain, warm, short. Sentence case. No jargon ("clip", not "content item").
- All strings in String Catalogs; English only in v0 but nothing hard-coded.
- Language names come from `Locale`, shown in the user's UI language.
- Support leading/trailing layout (RTL languages like Arabic are in the 15).
- Respect Dynamic Type and VoiceOver on the bar and all Me screens; the player itself is YouTube's.

## When asked to design something

1. Restate which phase it belongs to (v0 or later) and the user's goal.
2. Check the YouTube constraints above and the two-tab structure.
3. Give the flow as numbered steps, then each screen: content, actions, empty state, error state, loading state.
4. Name the event(s) it should log if it affects ranking (impression, view_end, share, player_error, or like/save/hide/block calls).
5. Call out anything that needs a spec change rather than inventing it silently.

Source of truth: the spec doc's "v0 build" tab (https://claude.ai/artifact/54gzKQZdV44NukYnP1DikG), modules V5 to V7.
