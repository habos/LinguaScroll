import UIKit
import WebKit

/// YouTube IFrame player states (YT.PlayerState) plus errors.
public enum YTState: Equatable, CustomStringConvertible {
    case unstarted, ended, playing, paused, buffering, cued, error(Int)

    init(raw: Int) {
        switch raw {
        case 0: self = .ended
        case 1: self = .playing
        case 2: self = .paused
        case 3: self = .buffering
        case 5: self = .cued
        default: self = .unstarted
        }
    }

    public var description: String {
        switch self {
        case .unstarted: "unstarted"
        case .ended: "ended"
        case .playing: "playing"
        case .paused: "paused"
        case .buffering: "buffering"
        case .cued: "cued"
        case .error(let c): "error \(c)"
        }
    }
}

/// One pooled WKWebView with the YouTube IFrame API loaded once.
/// Videos are swapped with cueVideoById / loadVideoById; the page is never reloaded.
@MainActor
public final class YouTubePlayer: NSObject, WKScriptMessageHandler {
    let webView: WKWebView
    let name: String
    private(set) var videoID: String?
    private(set) var state: YTState = .unstarted
    private(set) var isReady = false
    /// What the page was loaded with (QA V6-10 checks it).
    public private(set) var baseURL: URL?
    private var pendingJS: [String] = []

    /// (player, videoID, state, positionMs)
    var onState: ((YouTubePlayer, String?, YTState, Int) -> Void)?

    /// The page's origin, which becomes the Referer YouTube requires: https://<bundle id lowercased>.
    public static func origin(bundleID: String? = Bundle.main.bundleIdentifier) -> URL {
        URL(string: "https://\((bundleID ?? "app.linguascroll.ios").lowercased())")!
    }

    /// `captionsLanguage`: YouTube's own captions in that language (`cc_load_policy=1`), or nil for off.
    /// All clips in one feed share a language, so this is fixed per player.
    /// `controls`: YouTube's progress bar and title (`controls=1`); off shows only the video.
    init(name: String, captionsLanguage: String?, controls: Bool = false) {
        self.name = name
        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []
        let controller = WKUserContentController()
        config.userContentController = controller
        webView = WKWebView(frame: .zero, configuration: config)
        super.init()
        controller.add(WeakMessageHandler(self), name: "yt")

        webView.isOpaque = false
        webView.backgroundColor = .black
        webView.scrollView.backgroundColor = .black
        // The spike's key setting: the web view must not scroll itself, so a
        // vertical pan can belong to the feed's collection view.
        webView.scrollView.isScrollEnabled = false
        webView.scrollView.bounces = false
        webView.scrollView.contentInsetAdjustmentBehavior = .never
        if #available(iOS 16.4, *) { webView.isInspectable = true }

        let origin = Self.origin()
        baseURL = origin
        webView.loadHTMLString(Self.html(origin: origin.absoluteString, captionsLanguage: captionsLanguage, controls: controls), baseURL: origin)
    }

    // MARK: Commands

    /// Load the video paused and silent (thumbnail shows, nothing plays).
    func cue(_ id: String) {
        guard id != videoID else { return }
        videoID = id
        state = .unstarted
        run("cue('\(id)')")
    }

    /// Start playing `id` (cueing first if it isn't the loaded one).
    func play(_ id: String) {
        if id != videoID {
            videoID = id
            state = .unstarted
            run("load('\(id)')")
        } else {
            run("play()")
        }
    }

    func pause() { run("pause()") }
    func seekToStartAndPlay() { run("restart()") }

    private func run(_ js: String) {
        guard isReady else { pendingJS.append(js); return }
        webView.evaluateJavaScript(js, completionHandler: nil)
    }

    // MARK: Messages from the page

    public func userContentController(_ c: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let type = body["type"] as? String else { return }
        let pos = (body["t"] as? NSNumber)?.intValue ?? 0
        switch type {
        case "ready":
            isReady = true
            let queued = pendingJS
            pendingJS = []
            queued.forEach { webView.evaluateJavaScript($0, completionHandler: nil) }
        case "state":
            state = YTState(raw: (body["state"] as? NSNumber)?.intValue ?? -1)
            onState?(self, videoID, state, pos)
        case "error":
            state = .error((body["code"] as? NSNumber)?.intValue ?? -1)
            onState?(self, videoID, state, pos)
        default:
            break
        }
    }

    // MARK: Page

    static func html(origin: String, captionsLanguage: String?, controls: Bool) -> String {
        let captionVars = captionsLanguage.map { ", cc_load_policy: 1, cc_lang_pref: '\($0)'" } ?? ", cc_load_policy: 0"
        return """
        <!doctype html>
        <html><head>
        <meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no">
        <style>
          html,body{margin:0;padding:0;width:100%;height:100%;background:#000;overflow:hidden}
          #player{position:absolute;top:0;left:0;width:100%;height:100%}
        </style>
        </head><body>
        <div id="player"></div>
        <script>
          var player = null;
          function post(m){ window.webkit.messageHandlers.yt.postMessage(m); }
          function pos(){ try { return Math.round(player.getCurrentTime()*1000); } catch(e) { return 0; } }
          function onYouTubeIframeAPIReady(){
            player = new YT.Player('player', {
              width: '100%', height: '100%',
              playerVars: { playsinline: 1, rel: 0, controls: \(controls ? 1 : 0), fs: 0, iv_load_policy: 3, origin: '\(origin)'\(captionVars) },
              events: {
                onReady: function(){ post({type:'ready'}); },
                onStateChange: function(e){ post({type:'state', state:e.data, t:pos()}); },
                onError: function(e){ post({type:'error', code:e.data, t:pos()}); }
              }
            });
          }
          function cue(id){ player.cueVideoById(id); }
          function load(id){ player.loadVideoById(id); }
          function play(){ player.playVideo(); }
          function pause(){ player.pauseVideo(); }
          function restart(){ player.seekTo(0, true); player.playVideo(); }
        </script>
        <script src="https://www.youtube.com/iframe_api"></script>
        </body></html>
        """
    }
}

/// WKUserContentController retains its handlers; this breaks the cycle.
@MainActor
private final class WeakMessageHandler: NSObject, WKScriptMessageHandler {
    weak var target: (any WKScriptMessageHandler)?
    init(_ target: any WKScriptMessageHandler) { self.target = target }
    func userContentController(_ c: WKUserContentController, didReceive m: WKScriptMessage) {
        target?.userContentController(c, didReceive: m)
    }
}
