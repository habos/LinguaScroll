import Core
import Foundation
import Observation

/// Everything the screens need from outside, as protocols so previews and tests use fakes.
public struct AppDependencies: Sendable {
    public var keychain: any KeychainStore
    public var session: SessionManager
    public var catalog: any CatalogAPI
    public var users: any UserAPI
    public var feed: any FeedAPI
    public var events: BatchingEventLogger

    public init(keychain: any KeychainStore, session: SessionManager, catalog: any CatalogAPI, users: any UserAPI, feed: any FeedAPI, events: BatchingEventLogger) {
        self.keychain = keychain
        self.session = session
        self.catalog = catalog
        self.users = users
        self.feed = feed
        self.events = events
    }
}

@MainActor
@Observable
public final class AppModel {
    public enum Phase: Equatable { case launching, onboarding, blocked, ready }
    public enum Tab: Hashable { case feed, me }

    public private(set) var phase: Phase = .launching
    public private(set) var user: UserProfile?
    public var activeLanguage: String = ""
    public private(set) var config: AppConfig = .fallback
    public private(set) var languages: [LanguageOption] = []
    public private(set) var languagesFailed = false
    public var isAgeBlocked: Bool { phase == .blocked }
    public var selectedTab: Tab = .feed
    /// YouTube's own captions in the player (Settings). Defaults to the server config.
    public var captionsOn: Bool {
        get { access(keyPath: \.captionsOn); return defaults.object(forKey: Self.captionsKey) as? Bool ?? config.youtubeCaptionsDefault }
        set { withMutation(keyPath: \.captionsOn) { defaults.set(newValue, forKey: Self.captionsKey) } }
    }
    public var hiddenChannels: HiddenChannels { HiddenChannels(defaults: defaults) }

    public let deps: AppDependencies
    private let defaults: UserDefaults
    private static let cachedUserKey = "cached_user"
    static let captionsKey = "youtube_captions"

    public init(deps: AppDependencies, defaults: UserDefaults = .standard) {
        self.deps = deps
        self.defaults = defaults
    }

    /// Learnable languages in launch order.
    public var learnable: [LanguageOption] {
        languages.filter(\.isLearnable).sorted { $0.launchOrder < $1.launchOrder }
    }

    public var nativeOptions: [LanguageOption] {
        languages.filter(\.isNativeOption)
    }

    public func language(_ code: String) -> LanguageOption? {
        languages.first { $0.code == code }
    }

    /// Launch: age block first, then a known install goes straight to the feed.
    public func launch() async {
        if AgeBlock.isBlocked(in: deps.keychain) {
            phase = .blocked
            return
        }
        async let config = try? deps.catalog.config()
        async let langs = try? deps.catalog.languages()

        if await deps.session.isRegistered {
            if let cached = cachedUser { apply(cached) }
            phase = .ready
            await refreshUser()
        } else {
            phase = .onboarding
        }
        if let c = await config { self.config = c }
        if let l = await langs { languages = l } else { languagesFailed = true }
    }

    public func reloadLanguages() async {
        languagesFailed = false
        do { languages = try await deps.catalog.languages() } catch { languagesFailed = true }
    }

    /// Onboarding's last step. Throws `.ageBlocked` after switching to the block screen.
    public func register(_ answers: OnboardingAnswers) async throws {
        do {
            let user = try await deps.session.register(answers)
            apply(user)
            phase = .ready
        } catch APIError.ageBlocked {
            phase = .blocked
            throw APIError.ageBlocked
        }
    }

    public func refreshUser() async {
        let users = deps.users
        if let fresh = try? await deps.session.authorized({ try await users.me(token: $0) }) {
            apply(fresh)
        }
    }

    /// Makes `code` the feed language (optimistic; the server is told in the background).
    public func switchLanguage(to code: String) {
        guard code != activeLanguage, var user, let level = user.learning.first(where: { $0.language == code })?.level else { return }
        for i in user.learning.indices { user.learning[i].isActive = user.learning[i].language == code }
        apply(user)
        let users = deps.users
        Task {
            if let fresh = try? await deps.session.authorized({ try await users.putLanguage(code, level: level, active: true, token: $0) }) {
                apply(fresh)
            }
        }
    }

    // MARK: Me (V7)

    /// Level change: shown at once, `PUT /v1/me/languages/{code}` in the background; the next feed page uses it.
    public func setLevel(_ level: Int, for code: String) {
        guard var user, let i = user.learning.firstIndex(where: { $0.language == code }), user.learning[i].level != level else { return }
        user.learning[i].level = level
        apply(user)
        let users = deps.users
        Task {
            if let fresh = try? await deps.session.authorized({ try await users.putLanguage(code, level: level, active: nil, token: $0) }) {
                apply(fresh)
            }
        }
    }

    /// Adds a language and makes it the one the feed shows.
    public func addLanguage(_ code: String, level: Int) async throws {
        let users = deps.users
        let fresh = try await deps.session.authorized { try await users.putLanguage(code, level: level, active: true, token: $0) }
        apply(fresh)
        activeLanguage = code
    }

    /// Never the only language (the button is hidden then; the server also answers 422).
    public func removeLanguage(_ code: String) async throws {
        guard let user, user.learning.count > 1 else { return }
        let users = deps.users
        let fresh = try await deps.session.authorized { try await users.removeLanguage(code, token: $0) }
        apply(fresh)
    }

    public func setNativeLanguage(_ code: String) async throws {
        let users = deps.users
        let fresh = try await deps.session.authorized { try await users.setNativeLanguage(code, token: $0) }
        apply(fresh)
    }

    /// Reset my data: `DELETE /v1/me`, then forget the token, install ID, cached user, local lists and
    /// queued events, and go back to onboarding. The age block stays (SessionManager.reset keeps it).
    public func resetData() async throws {
        let users = deps.users
        try await deps.session.authorized { try await users.deleteMe(token: $0) }
        await deps.session.reset()
        await deps.events.discardAll()
        defaults.removeObject(forKey: Self.cachedUserKey)
        defaults.removeObject(forKey: Self.captionsKey)
        defaults.removeObject(forKey: FeedModel.firstLikeKey)
        hiddenChannels.clear()
        user = nil
        activeLanguage = ""
        selectedTab = .feed
        phase = .onboarding
    }

    private func apply(_ user: UserProfile) {
        self.user = user
        if let lang = user.activeLanguage { activeLanguage = lang }
        if let data = try? JSONEncoder().encode(user) { defaults.set(data, forKey: Self.cachedUserKey) }
    }

    private var cachedUser: UserProfile? {
        defaults.data(forKey: Self.cachedUserKey).flatMap { try? JSONDecoder().decode(UserProfile.self, from: $0) }
    }
}
