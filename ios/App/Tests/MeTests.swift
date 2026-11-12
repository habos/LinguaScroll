import Core
@testable import Features
import XCTest

/// Fake server for the V7 tests: records every call, answers with one adult user learning es (B1) and ja (A1).
actor FakeAPI: SessionAPI, CatalogAPI, UserAPI, FeedAPI {
    private(set) var calls: [String] = []
    var user = UserProfile(id: "u1", ageBand: "adult", uiLanguage: "en", nativeLanguage: "en", learning: [
        LearningLanguage(language: "es", level: 3, isActive: true),
        LearningLanguage(language: "ja", level: 1, isActive: false),
    ])

    func createSession(_ r: SessionRequest) async throws -> SessionResult { calls.append("session"); return SessionResult(token: "t", user: user) }
    func config() async throws -> AppConfig { .fallback }
    func languages() async throws -> [LanguageOption] { [] }
    func me(token: String) async throws -> UserProfile { user }
    func putLanguage(_ code: String, level: Int, active: Bool?, token: String) async throws -> UserProfile {
        calls.append("put \(code) \(level) \(active.map { "\($0)" } ?? "nil")")
        if let i = user.learning.firstIndex(where: { $0.language == code }) { user.learning[i].level = level }
        return user
    }
    func removeLanguage(_ code: String, token: String) async throws -> UserProfile {
        calls.append("remove \(code)")
        user.learning.removeAll { $0.language == code }
        return user
    }
    func setNativeLanguage(_ code: String, token: String) async throws -> UserProfile { calls.append("native \(code)"); return user }
    func deleteMe(token: String) async throws { calls.append("deleteMe") }
    func feed(language: String, cursor: String?, token: String) async throws -> FeedPage { calls.append("feed \(language)"); return FeedPage(items: [], nextCursor: nil) }
    func setLiked(_ liked: Bool, videoID: String, token: String) async throws { calls.append("liked \(liked) \(videoID)") }
    func setSaved(_ saved: Bool, videoID: String, token: String) async throws { calls.append("saved \(saved) \(videoID)") }
    func hide(videoID: String, token: String) async throws { calls.append("hide \(videoID)") }
    func setChannelBlocked(_ blocked: Bool, channelID: String, token: String) async throws { calls.append("block \(blocked) \(channelID)") }
    func report(videoID: String, reason: ReportReason, token: String) async throws { calls.append("report \(videoID)") }
    func library(_ kind: LibraryKind, cursor: String?, token: String) async throws -> FeedPage {
        calls.append("library \(kind.rawValue) \(cursor ?? "nil")")
        return FeedPage(items: [], nextCursor: nil)
    }
}

private struct NoSink: EventSink {
    func send(_ events: [ClientEvent]) async throws {}
}

@MainActor
final class MeTests: XCTestCase {
    private var api: FakeAPI!
    private var keychain: InMemoryKeychain!
    private var defaults: UserDefaults!
    private var deps: AppDependencies!

    override func setUp() async throws {
        api = FakeAPI()
        keychain = InMemoryKeychain()
        defaults = UserDefaults(suiteName: "me-tests-\(UUID().uuidString)")!
        deps = AppDependencies(keychain: keychain, session: SessionManager(api: api, keychain: keychain, uiLanguage: "en"),
                               catalog: api, users: api, feed: api, events: BatchingEventLogger(sink: NoSink(), fileURL: nil))
    }

    private func registeredApp() async throws -> AppModel {
        let app = AppModel(deps: deps, defaults: defaults)
        try await app.register(OnboardingAnswers(birthYear: 1990, learning: "es", native: "en", level: 3))
        return app
    }

    private func card(_ n: Int, channel: String = "UC1") -> VideoCard {
        VideoCard(id: "00000000-0000-0000-0000-00000000000\(n)", language: "es", level: 3, durationMs: 20_000, aspect: 9.0 / 16.0,
                  youtubeID: "abcdefghij\(n)", title: "t\(n)", channelID: channel, channelTitle: "Channel \(channel)",
                  hasCaptions: true, liked: false, saved: true)
    }

    private func eventually(_ check: () async -> Bool) async {
        for _ in 0..<100 {
            if await check() { return }
            try? await Task.sleep(for: .milliseconds(20))
        }
    }

    // V7-03: changing Spanish's level shows at once and is sent with PUT /v1/me/languages/es (active untouched).
    func testChangingLevelUpdatesUserAndCallsPut() async throws {
        let app = try await registeredApp()
        app.setLevel(5, for: "es")
        XCTAssertEqual(app.user?.learning.first { $0.language == "es" }?.level, 5)
        await eventually { await self.api.calls.contains("put es 5 nil") }
        let calls = await api.calls
        XCTAssertTrue(calls.contains("put es 5 nil"), "\(calls)")
    }

    // V7-02: a Saved list opens on the tapped clip and pages with /v1/me/saves, never the ranked feed.
    func testLibraryPlaysFromTappedClipAndPagesItsOwnList() async throws {
        _ = try await registeredApp() // a session token, as in the app
        let model = FeedModel(library: .saved, cards: [card(1), card(2), card(3)], cursor: "c1", startIndex: 2,
                              deps: deps, captionsOn: false, defaults: defaults)
        XCTAssertEqual(model.currentIndex, 2)
        XCTAssertEqual(model.current?.id, card(3).id)
        XCTAssertEqual(model.state, .ready)
        await model.loadMore()
        let calls = await api.calls
        XCTAssertEqual(calls, ["session", "library saved c1"])
    }

    // V7-04 (automated half): reset calls DELETE /v1/me, forgets token and install ID, keeps the age block, goes to onboarding.
    func testResetErasesAndReturnsToOnboarding() async throws {
        let app = try await registeredApp()
        AgeBlock.set(in: keychain)
        app.hiddenChannels.add("UC9", title: "x")
        try await app.resetData()
        let calls = await api.calls
        XCTAssertTrue(calls.contains("deleteMe"))
        XCTAssertEqual(app.phase, .onboarding)
        XCTAssertNil(app.user)
        XCTAssertNil(keychain.string(for: KeychainKey.sessionToken))
        XCTAssertNil(keychain.string(for: KeychainKey.installID))
        XCTAssertTrue(AgeBlock.isBlocked(in: keychain))
        XCTAssertTrue(app.hiddenChannels.all.isEmpty)
    }

    // V7-05: hiding a channel lists it under Hidden channels; Undo takes it off again.
    func testBlockedChannelIsListedAndUndoRemovesIt() {
        let model = FeedModel(library: .saved, cards: [card(1, channel: "UC7"), card(2)], cursor: nil, startIndex: 0,
                              deps: deps, captionsOn: false, defaults: defaults)
        model.blockChannel(card(1, channel: "UC7"))
        let hidden = HiddenChannels(defaults: defaults)
        XCTAssertEqual(hidden.all.map(\.id), ["UC7"])
        XCTAssertEqual(hidden.all.first?.title, "Channel UC7")
        model.undo()
        XCTAssertTrue(hidden.all.isEmpty)
    }

    // V7-06: thumbnails come from YouTube's public URL and are never cached to disk.
    func testThumbnailsAreNotStoredOnDisk() {
        XCTAssertEqual(card(1).thumbnailURL.host, "i.ytimg.com")
        XCTAssertEqual(Thumbnails.session.configuration.urlCache?.diskCapacity ?? 0, 0)
    }

    // V7-07: captions off leaves cc_load_policy out of the player; on sets it with the clip language.
    func testCaptionsOffLeavesCCLoadPolicyOut() {
        let off = YouTubePlayer.html(origin: "https://app.linguascroll.ios", captionsLanguage: nil, controls: false)
        XCTAssertFalse(off.contains("cc_load_policy"))
        let on = YouTubePlayer.html(origin: "https://app.linguascroll.ios", captionsLanguage: "es", controls: false)
        XCTAssertTrue(on.contains("cc_load_policy: 1"))
        XCTAssertTrue(on.contains("cc_lang_pref: 'es'"))
    }

    // Captions setting defaults to the server config and remembers the user's choice.
    func testCaptionsSettingPersists() {
        let app = AppModel(deps: deps, defaults: defaults)
        XCTAssertEqual(app.captionsOn, AppConfig.fallback.youtubeCaptionsDefault)
        app.captionsOn = false
        XCTAssertFalse(AppModel(deps: deps, defaults: defaults).captionsOn)
    }
}
