# LinguaScroll v0 UX spec

The screens for the YouTube-only v0: onboarding, the feed and the Me tab (modules V5, V6 and V7 of the spec).
Visual design with every screen: https://claude.ai/artifact/MiJ23BiGKzwL1GsXvmy4Xt (click Play on an artboard to walk the flow).
Rules this follows: `.claude/skills/linguascroll-ux` and `.claude/skills/linguascroll-youtube-rules`.

Where this file and the spec's module tabs disagree, the section "Changes to the spec" at the end says which wins and why.

## 1. Look

v0 is dark only (`.preferredColorScheme(.dark)`). A video app reads best on black, and it halves the work.

| Token | Value | Use |
|---|---|---|
| `bg` | `#000000` | Every screen, feed cell, the space around the player |
| `surface` | `#161618` | Cards and list groups |
| `surfaceRaised` | `#1C1C1F` | Sheets, language pill, search field |
| `fill` | `#2A2A2E` | Secondary buttons, separators (`#1F1F22` hairlines) |
| `textPrimary` | `#F5F5F7` | |
| `textSecondary` | `#A1A1AA` | Subtitles, footnotes (7.9:1 on black) |
| `textTertiary` | `#71717A` | Version line, chevrons |
| `accent` | `#FF6A4D` | Primary buttons, liked/saved fill, selected states, links |
| `onAccent` | `#1A0905` | Text on accent buttons |
| `destructive` | `#FF8A73` | Report, Remove, Reset, error lines |

Type: the system font (SF Pro) with Dynamic Type styles, no custom font. Mockups use Inter as a stand-in.

| Style | Where |
|---|---|
| `.largeTitle` bold (30 pt in mockups) | Onboarding questions, "Me" |
| `.title3` semibold | Sheet titles, empty-state titles |
| `.body` / `.body` semibold | Rows, buttons, channel name (15 pt in bar = `.subheadline` semibold) |
| `.footnote` | Hints and footers |
| `.caption` | "Open on YouTube", tab labels |

Shapes: cards and buttons 14 pt corners, sheets 22 pt, pills fully round. Primary buttons 52 pt tall. All touch targets at least 44x44 pt. Margins 16 pt (24 pt for onboarding text).

## 2. App structure

- First launch: onboarding (4 screens), then the Feed tab.
- Two tabs: **Feed** (`play.rectangle.fill`) and **Me** (`person.crop.circle`). Tab bar is black with a `#1F1F22` hairline; selected `textPrimary`, unselected `#8E8E96`.
- No sign-in, no permission prompts, no notifications, no badges, no streaks.
- If the Keychain says the device is age-blocked, show only the block screen on every launch.

## 3. Onboarding (V5)

Nothing is sent until the last answer. Then `POST /v1/sessions/anonymous` with all four answers, and the feed's first page is requested right away. Target: under 30 seconds of taps.

Each screen has a back chevron (`chevron.left`, except screen 1) and "N of 4" at the trailing edge of the top bar.

| # | Screen | Content | How it advances |
|---|---|---|---|
| 1 | Birth year | Wordmark; "Learn a language by scrolling." / "First, what year were you born?"; year wheel defaulting to (current year − 20), range current year back 100 years; footnote "We use this only to check your age. We keep an age range, not the year." | Continue button |
| 2 | Language to learn | "Which language do you want to learn?"; one row per learnable language from `GET /v1/languages` in launch order: name in the UI language (`Locale`) leading, endonym trailing in `textSecondary`. No flags. | Tap a row (no Continue button) |
| 3 | Native language | "What's your native language?" / "We picked it from your phone's settings."; the device language preselected at top (accent border, `checkmark`); search field; all `Locale` languages A to Z, minus the one chosen in step 2 | Continue, or tap a row |
| 4 | Level | "How much {language} do you know?"; three cards: **Just starting** "I know a few words and phrases." (level 1), **Can get by** "I can follow slow, simple conversations." (3), **Pretty comfortable** "I understand most everyday talk." (5); footnote "A rough guess is fine. It only steers which clips you see first, and you can change it in Me." | Tap a card: register, then feed |

States:
- Registering: the tapped card shows a small spinner; other cards disabled. Usually under a second, so no extra screen.
- Registration fails (offline or 5xx): stay on screen 4, inline line under the cards in `destructive`: "Couldn't connect. Check your connection and tap again." Answers are kept.
- `403 age_blocked`: store the block in the Keychain and replace onboarding with the block screen.

**Under-13 screen:** centered wordmark, "LinguaScroll is for ages 13 and up", "Thanks for stopping by." No buttons, no hint of the cut-off, no way back.

## 4. Feed (V6)

### Layout

```
safe area top (system)
top bar 44 pt         language pill, leading
player area           player at the clip's aspect, as large as fits, centred, black around it
action bar 64 pt min  channel | like save share more   (pinned)
tab bar               system
```

- **The player fits; it does not fill the width.** On every iPhone a 9:16 player is too tall for the width once the bars are in. iPhone 15: 393x602 available, so the player is 339x602. iPhone SE: 276x490. 4:5 clips go full width with black above and below.
- The WKWebView frame always equals the clip's aspect (from `VideoCard.aspect`), so YouTube doesn't letterbox inside it. Never crop, scale-to-fill or stretch.
- The action bar is pinned above the tab bar so the buttons never move between clips. Only the player area pages.
- Nothing of ours inside the player frame. No gesture recognisers that act on taps, double-taps or long-presses there. Vertical swipes page the feed (depends on the V6 day-one spike; if it fails, a swipe strip goes in the bar and this section changes).
- Accessibility text sizes: the bar grows to fit its text; the player shrinks to fit, never below 200x200.
- Use leading/trailing everywhere so a right-to-left interface mirrors it.

### Top bar

Language pill (`surfaceRaised`, 34 pt tall, round): "{Language}" semibold + " · {CEFR}" in `textSecondary` + `chevron.down`. Tap opens the language switcher sheet.

When playing a Saved or Liked list (opened from Me), the pill is replaced by `chevron.left` + "Saved" / "Liked", which pops back to Me. Everything else is the same.

### Action bar

| Item | SF Symbol | Off / on | VoiceOver label | Action |
|---|---|---|---|---|
| Channel | `arrow.up.right` (12 pt, after "Open on YouTube") | | "{channel}, opens in YouTube" | Open `https://www.youtube.com/channel/{channel_id}` (universal link opens the YouTube app if installed, else Safari). Pause first. |
| Like | `heart` / `heart.fill` | `textPrimary` / `accent` | "Like" / "Liked" (selected trait) | `PUT`/`DELETE /v1/videos/{id}/like`, optimistic, light haptic when turning on |
| Save | `bookmark` / `bookmark.fill` | `textPrimary` / `accent` | "Save" / "Saved" | `PUT`/`DELETE .../save`, optimistic, light haptic when turning on |
| Share | `square.and.arrow.up` | | "Share" | Share sheet with `https://youtube.com/shorts/{video_id}`; log `share` |
| More | `ellipsis` | | "More options" | More sheet |

Channel column: channel title (`.subheadline` semibold, one line, truncates) over "Open on YouTube ↗" (`.caption`, `textSecondary`). Icons 44x44 hit areas, 4 pt apart, 8 pt trailing inset, 16 pt leading.

**Bar messages** replace the channel column for 2.5 s, then the channel comes back. They never float over the player. Post them as VoiceOver announcements too.

| When | Line 1 | Line 2 |
|---|---|---|
| First like on this install (once, flag in UserDefaults) | Liked | Likes shape what you see next |
| Like/save/hide call failed (rolled back) | Couldn't save that (`destructive`) | Check your connection and try again |
| Player error | This clip can't play here | Skipping to the next one… |
| After hide or channel block (shown on the next clip) | Hidden | Undo (button) |

### Sheets

**Any sheet or the share sheet pauses the clip before it opens and resumes when it closes.** Sheets cover part of the player, so it must not be playing under them.

More sheet (`surfaceRaised`, grabber, rows 60 pt):
1. `eye.slash` **Hide this clip** / "You won't see it again" → `PUT /hide`, cell animates away, feed advances.
2. `nosign` **Not interested in {channel}** / "Hide clips from this channel. Undo in Settings" → `PUT /v1/channel-blocks/{channel_id}`, remove that channel's later cells already loaded, advance.
3. `flag` **Report** (`destructive`) / "Not in {language}, not appropriate, spam, other" → second sheet with four reasons: "Not in {language}", "Not appropriate", "Spam or misleading", "Something else". Picking one calls `POST /report`, hides the clip for this device and advances. No free text in v0.
4. Cancel.

Language switcher sheet: title "Watch clips in"; one row per learner language (name, "{CEFR} · {plain level}"), `checkmark` on the active one; tap another to switch; "Add a language" (`plus`, accent) opens the add-language flow (screen 2 then 4 of onboarding, as a sheet); "Manage languages and levels" opens Me › Languages.

Switching language: dismiss the sheet, crossfade the feed to the loading state, request `GET /v1/feed?language=` from no cursor, start at the top. The old language's position isn't kept in v0.

### Playback and states

- Play only when the cell is more than half visible; one player plays; loop on `ended`.
- Before a clip's player reaches `playing`, show **our own placeholder in the player's frame instead of the player** (`#111113`, plus the YouTube thumbnail `https://i.ytimg.com/vi/{id}/hqdefault.jpg` if it loads). Swap, never stack: the web view is hidden while the placeholder shows, and the placeholder is removed when the player shows.
- First load: placeholder with a spinner and "Finding {language} clips…"; bar shows skeleton shapes.
- Offline / feed request fails: "You're offline" / "We'll carry on as soon as you're back." / **Try again** button. Retry automatically when the network comes back. Clips already loaded keep playing.
- Player error: bar message, `player_error` event, auto-advance after 1 s (codes 101/150 also `PUT /hide`).
- Out of clips (should be rare, see open questions): "You've seen every {language} clip we have for now" / "New clips arrive every day. Meanwhile:" / **Rewatch saved clips**, **Switch language**.

### Events

`impression` when a cell becomes current; `view_end` when it stops being current or the app backgrounds; `share` on share sheet open; `player_error` on error. Like, save, hide, block and report are their own API calls. Opening a sheet does not end the view.

## 5. Me (V7)

Me screen:
- Header: "Me" large title, `gearshape` button (trailing) → Settings.
- "Learning" card: "Spanish B1 · Japanese A1", `chevron.right` → Languages.
- Segmented control Saved | Liked (remember the last choice per session).
- 3-column grid, 2 pt gaps, cells at 9:16 from the YouTube thumbnail URL (no images stored); duration bottom-leading and language code top-trailing (only when the device has more than one language), both on a 55% black chip. Newest first, loads more at the end.
- Tap a cell: play that list from that clip in the feed layout (back button in the top bar).
- Long-press a cell (our thumbnail, not the player): context menu Unsave / Unlike, Share.
- Empty Saved: `bookmark` icon, "Nothing saved yet", "Tap the bookmark under a clip to keep it here."
- Empty Liked: `heart` icon, "No likes yet", "Clips you like show up here. Likes also tune your feed."
- Load error: "Couldn't load your clips" + Try again, in place of the grid.

Languages screen:
- One card per language. The active one says "Watching now" with a `checkmark`; it's expanded with six level chips A1 to C2 (radio group, selected = accent) and a plain line, e.g. "B1 · Can get by: slow, simple conversations"; "Remove {language}" in `destructive` (hidden when it's the only language).
- Other languages: collapsed row with "{CEFR} · {plain level}" and a **Watch** button that makes it active and jumps to the feed. Tap the row to expand it.
- "Add a language" card → language list (only languages not added yet) → level cards → back here with the new one active.
- Footer: "Your level is a rough guide that steers which clips you see. Nothing is graded. You can't remove your only language."
- Level words for each number: 1 A1 "Just starting", 2 A2 "Know the basics", 3 B1 "Can get by", 4 B2 "Getting confident", 5 C1 "Pretty comfortable", 6 C2 "Near native".
- Changing level calls `PUT /v1/me/languages/{code}` immediately (no Save button); the next feed page uses it.

Settings screen (grouped list):
- Native language (`chevron.right`, value trailing) → same list as onboarding screen 3.
- YouTube captions toggle; footer "Shown by YouTube inside the player, in the clip's language, when the clip has them." Off sets `cc_load_policy=0` on the next clip.
- Hidden channels (count) → list of channel titles, each with "Show again" (`DELETE /v1/channel-blocks/{id}`). Empty: "No hidden channels."
- About your data → privacy screen.
- Reset my data (`destructive`) → privacy screen's confirm card: "Reset my data?" / "Your likes, saves, languages and watch history will be erased. This can't be undone." / Cancel, **Erase everything**. Then `DELETE /v1/me`, clear the Keychain token and install ID, return to onboarding. The age block is never cleared by reset.
- Footer: "LinguaScroll {version} ({build}) · Clips play in YouTube's player".

Privacy copy ("About your data"):

> There's no account. When you first opened LinguaScroll, it made a random ID that stays on this phone. It isn't linked to your name, email, Apple ID or advertising ID.
>
> **What we keep with that ID:** your age range, your languages and levels, the clips you like, save, hide or report, the channels you hid, and how long you watch each clip.
>
> **Why:** to pick the next clips for you. We don't sell it or use it for ads.
>
> **YouTube:** clips play in YouTube's own player, so YouTube's privacy policy covers what you watch there.
>
> On a new phone you start fresh. Resetting erases all of it from our server and this phone.

## 6. Changes to the spec

| # | Spec says | This design does | Why |
|---|---|---|---|
| 1 | V6: "The player fills the width at the clip's aspect, from the top safe area down" | Player is the largest rect at the clip's aspect that fits between top bar and action bar, centred; action bar pinned above the tab bar | 9:16 at full width doesn't fit with the bars on any iPhone |
| 2 | V4: report has a free `reason` | Four fixed reasons, the existing contract enum: `not_this_language`, `inappropriate`, `spam`, `other` (backend confirmed; a `not_this_language` report also flags the clip for a language re-check) | "Not in Spanish" is the most useful signal for catalog quality; needs backend enum |
| 3 | V4: only `PUT /v1/videos/{id}/hide` | Add `DELETE /v1/videos/{id}/hide` (204, idempotent; backend adds it in module V4) for the Undo after hiding | Undo is cheap and stops accidental hides from feeling final |
| 4 | V6: sheets not covered | Any sheet pauses the clip first | Sheets overlap the player; it mustn't play under them |
| 5 | V7: long-press to unsave or unlike | Same, as a context menu with Share too | Standard iOS pattern |

## 7. Open questions

- Feed thread: can the feed widen the level window and lower the explore threshold before it ever returns an empty page? The out-of-clips screen should be close to unreachable.
- The age block is permanent on the device (the Keychain survives reinstalls). Fine for v0; later it could expire once the stored age band reaches 13.
