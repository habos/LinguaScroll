# V6 day-one spike: swiping over the YouTube player

Throwaway app that answers one question before we build the real feed: **can a vertical swipe that starts on the embedded YouTube player page the feed, while taps still reach YouTube's own controls?** It uses the same setup the real feed will use: a paging `UICollectionView`, a pool of exactly two `WKWebView` IFrame players, the `https://<bundle id>` Referer, play only when more than half visible, cue the next clip, loop on end.

## Run it (real iPhone; the simulator doesn't count for touch feel)

```sh
brew install xcodegen          # once
cd ios/Spike && xcodegen && open LinguaScrollSpike.xcodeproj
```

1. Signing & Capabilities: pick your Personal Team. If the bundle ID is taken, change it to something like `com.<yourname>.linguascroll.spike`.
2. Plug in the iPhone, select it, Run. First time: on the phone, Settings > General > VPN & Device Management > trust your developer certificate. Developer Mode must be on (Settings > Privacy & Security).
3. In the app, tap **Links** and paste 10+ YouTube Shorts links (YouTube app: Share > Copy link), one per line. Ideally a mix of languages, some with captions.

## What to check

The top strip is a debug HUD (it is above the feed, never over the player).

| # | Test | Pass if |
|---|---|---|
| 1 | With **Swipe over player** on, swipe up starting in the middle of the video | Feed moves to the next clip |
| 2 | Same, but start the swipe on YouTube's controls (title, progress bar) | Feed still moves (or tell me what happens) |
| 3 | Tap the video once | YouTube's play/pause/controls respond; feed doesn't move |
| 4 | Tap the CC / settings buttons inside the player | They open |
| 5 | 30 fast swipes down then back up | `2xAudio` stays 0 and you never hear two clips |
| 6 | Drag a clip halfway and hold | It pauses as soon as half of it is gone; let go and it resumes |
| 7 | Watch `ttff` / `med` after ~20 swipes on Wi-Fi | Median under 800 ms is the target |
| 8 | Lock the phone / go home | Audio stops |
| 9 | Toggle **Swipe over player** off and repeat 1 | Tells us whether the override is what makes it work (swiping on the bar should work either way) |
| 10 | Xcode > Debug navigator > Memory during 50 swipes | Stays flat |

If 1 or 3 fails, the fallback in the spec is a swipe strip in the bar, which feels worse; tell me exactly what happened and I'll try the next approach before we fall back.
