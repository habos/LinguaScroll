import Core
@testable import Features
import UIKit
import WebKit
import XCTest

/// QA matrix rows V6-05, V6-08, V6-09, V6-10. Run on the iPhone SE (3rd gen) simulator in CI.
@MainActor
final class FeedLayoutTests: XCTestCase {
    private func card(aspect: Double) -> VideoCard {
        VideoCard(id: "00000000-0000-0000-0000-000000000001", language: "es", level: 3, durationMs: 20_000,
                  aspect: aspect, youtubeID: "abcdefghijk", title: "t", channelID: "UC1", channelTitle: "c",
                  hasCaptions: true, liked: false, saved: false)
    }

    /// The feed's player area on this device: screen minus status bar, 44 pt top bar, 64 pt action bar, tab bar.
    private func playerArea() -> CGRect {
        let screen = UIScreen.main.bounds
        let window = UIApplication.shared.connectedScenes.compactMap { ($0 as? UIWindowScene)?.windows.first }.first
        let insets = window?.safeAreaInsets ?? .zero
        let tabBar: CGFloat = 49 + insets.bottom
        return CGRect(x: 0, y: 0, width: screen.width, height: screen.height - insets.top - 44 - ActionBar.minHeight - tabBar)
    }

    // V6-09: player at least 200x200 pt at every supported aspect, on this (smallest) device.
    func testPlayerIsAtLeast200x200() {
        for aspect in [9.0 / 16.0, 4.0 / 5.0, 1.0] {
            let frame = FeedCell.fit(aspect: CGFloat(aspect), in: playerArea())
            XCTAssertGreaterThanOrEqual(frame.width, FeedCell.minPlayerSide, "aspect \(aspect)")
            XCTAssertGreaterThanOrEqual(frame.height, FeedCell.minPlayerSide, "aspect \(aspect)")
        }
    }

    func testFitKeepsAspectAndCentres() {
        let frame = FeedCell.fit(aspect: 9.0 / 16.0, in: CGRect(x: 0, y: 0, width: 393, height: 602))
        XCTAssertEqual(frame.height, 602)
        XCTAssertEqual(frame.width, 339, accuracy: 1)
        XCTAssertEqual(frame.midX, 196.5, accuracy: 1)
    }

    // V6-05: nothing of ours overlaps the player frame once the player shows.
    func testNothingOverThePlayer() {
        let cell = FeedCell(frame: CGRect(origin: .zero, size: playerArea().size))
        cell.configure(card(aspect: 9.0 / 16.0))
        let webView = WKWebView()
        cell.host(webView)
        cell.showPlayer(true)
        cell.layoutIfNeeded()

        let visibleInFrame = cell.playerFrame.subviews.filter { !$0.isHidden && $0.alpha > 0.01 }
        XCTAssertEqual(visibleInFrame.map { ObjectIdentifier($0) }, [ObjectIdentifier(webView)],
                       "only the web view may be visible inside the player frame")
        // No sibling of the player frame overlaps it.
        for sibling in cell.contentView.subviews where sibling !== cell.playerFrame && !sibling.isHidden {
            XCTAssertFalse(sibling.frame.intersects(cell.playerFrame.frame), "\(sibling) overlaps the player")
        }
    }

    // Placeholder and player swap, never stack.
    func testPlaceholderSwapsWithPlayer() {
        let cell = FeedCell(frame: CGRect(x: 0, y: 0, width: 375, height: 500))
        cell.configure(card(aspect: 9.0 / 16.0))
        let webView = WKWebView()
        cell.host(webView)
        cell.showPlayer(false)
        let shown = cell.playerFrame.subviews.filter { !$0.isHidden && $0.alpha > 0.01 }
        XCTAssertEqual(shown.count, 1)
        XCTAssertFalse(shown.first === webView)
    }

    // V6-08: no gesture recognisers of ours on the player frame or the web view.
    func testNoGestureRecognisersOnPlayer() {
        let cell = FeedCell(frame: CGRect(x: 0, y: 0, width: 375, height: 500))
        cell.configure(card(aspect: 9.0 / 16.0))
        let webView = WKWebView()
        cell.host(webView)
        XCTAssertTrue(cell.playerFrame.gestureRecognizers?.isEmpty ?? true)
        // UIKit can attach its own recognisers to a cell's content view; none may act on taps,
        // double-taps or long-presses (that would be a touch action of ours on the player).
        let onContent = cell.contentView.gestureRecognizers ?? []
        XCTAssertFalse(onContent.contains { $0 is UITapGestureRecognizer || $0 is UILongPressGestureRecognizer },
                       "tap/long-press recognisers on the cell: \(onContent)")
        XCTAssertTrue(webView.gestureRecognizers?.isEmpty ?? true)
    }

    // V6-10: the IFrame page is loaded with baseURL https://<bundle id lowercased>.
    func testBaseURLIsBundleIDOrigin() {
        XCTAssertEqual(YouTubePlayer.origin(bundleID: "App.LinguaScroll.iOS").absoluteString, "https://app.linguascroll.ios")
        let player = YouTubePlayer(name: "T", captionsLanguage: "es")
        let expected = "https://" + (Bundle.main.bundleIdentifier ?? "").lowercased()
        XCTAssertEqual(player.baseURL?.absoluteString, expected)
    }
}
