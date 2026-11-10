# Real-device pass (every V6 change)

Needs a real iPhone; the simulator doesn't count for touch feel, audio or memory. For the day-one spike, use the 10-step table in `ios/Spike/README.md` (PR #2). This script is the same pass for the real app, plus the compliance steps.

Log the result in `qa/runs/<date>-<what>.md` with the commit SHA, device model, iOS version and network.

## Setup

1. Build the app on the device from Xcode (Personal Team signing).
2. Backend running with real clips (V3 run done), or at least 30 real Shorts in the target language.
3. Fresh install, onboard to Spanish level 1.

## Steps

| # | Step | Pass if | Matrix |
|---|---|---|---|
| 1 | Swipe up starting in the middle of the video | Feed moves to the next clip | V6-01 |
| 2 | Start the swipe on YouTube's controls (title, progress bar) | Feed still moves, or note exactly what happens | V6-01 |
| 3 | Tap the video once | YouTube's play/pause/controls respond; feed doesn't move; nothing of ours reacts (no like) | V6-01, Y2 |
| 4 | Tap CC and settings inside the player | They open; YouTube logo and controls visible | Y7 |
| 5 | 30 fast swipes down, then back up | Never two clips audible at once | V6-03, Y4 |
| 6 | Drag a clip halfway and hold | Pauses once less than half is visible; resumes on release | V6-04, Y3 |
| 7 | After ~20 swipes on Wi-Fi, read median `ttff_ms` from the event log (`SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY (props->>'ttff_ms')::int) FROM events WHERE type='view_end' AND at > now() - interval '15 min'`) | Under 800 ms is the target; record the number either way | V6-02 |
| 8 | Lock the phone, then go to Home | Audio stops both times | V6-12, Y4 |
| 9 | Xcode Debug navigator > Memory over 50 swipes | Flat (no steady climb); still 2 web views in the view debugger | V6-06 |
| 10 | Play a clip that has captions | YouTube's captions show in the clip language | V6-07 |
| 11 | Turn captions off in Settings, replay | No captions | V7-07 |
| 12 | Find or force a clip with error 101/150 (embedding disabled) | Auto-advances after ~1 s; never comes back after relaunch | V6-11 |
| 13 | Tap like in the bar with the backend stopped | Like shows, then rolls back with a toast | V6-13 |
| 14 | Share from the bar | Share sheet link is `https://youtube.com/shorts/<id>` | V6-14 |
| 15 | Tap the channel name | Opens the channel in the YouTube app or Safari | Y7 |
| 16 | View debugger: inspect the player cell | Nothing of ours inside the player frame | V6-05, Y1 |
