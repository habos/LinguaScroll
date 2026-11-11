# iOS install, reinstall and reset (V5, V7)

Simulator is fine for this one. Backend running locally (`make dev`).

## A. Fresh install (V5-01)

1. Erase the simulator (Device > Erase All Content and Settings) so the Keychain is empty.
2. Run the app, start a timer.
3. Onboard: birth year 2000, learn Spanish, native English, "just starting".
4. **Pass:** feed (or placeholder) shows in under 30 s. Note the user ID (`SELECT id, install_id FROM users ORDER BY created_at DESC LIMIT 1`).

## B. Reinstall keeps identity (V5-02)

1. Like 2 clips.
2. Delete the app (long-press > Remove App), reinstall from Xcode.
3. **Pass:** no onboarding; same user ID in the DB (no new `users` row); the 2 likes are in Liked.

## C. Age block survives reinstall (V5-03)

1. Erase the simulator. Run, enter birth year 2016.
2. **Pass:** calm "LinguaScroll is for ages 13 and up" screen; no `users` row created.
3. Kill and relaunch: blocked screen again.
4. Delete and reinstall: blocked screen again (stored in Keychain).

## D. Events after a kill (V5-04)

1. With the feed open, watch 3 clips, then kill the app within 10 s (before the flush).
2. Relaunch.
3. **Pass:** the pending `impression`/`view_end` events arrive once each (`SELECT id, count(*) FROM events GROUP BY id HAVING count(*) > 1` returns nothing; the 3 clips' events are present).

## E. Reset my data (V7-04)

1. Like a clip. Me > Settings > Reset my data, confirm twice.
2. **Pass:** back at onboarding; old user row gone; after onboarding again Liked is empty; the old token gets 401.
