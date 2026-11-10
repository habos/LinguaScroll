import Core
import Foundation
import Observation

/// Everything the screens need from outside, as protocols so previews and tests use fakes.
public struct AppDependencies: Sendable {
    public var keychain: any KeychainStore
    public var session: SessionManager
    public var catalog: any CatalogAPI
    public var users: any UserAPI
    public var events: BatchingEventLogger

    public init(keychain: any KeychainStore, session: SessionManager, catalog: any CatalogAPI, users: any UserAPI, events: BatchingEventLogger) {
        self.keychain = keychain
        self.session = session
        self.catalog = catalog
        self.users = users
        self.events = events
    }
}

@MainActor
@Observable
public final class AppModel {
    public enum Phase: Equatable { case launching, onboarding, blocked, ready }

    public private(set) var phase: Phase = .launching
    public private(set) var user: UserProfile?
    public var activeLanguage: String = ""
    public private(set) var config: AppConfig = .fallback
    public private(set) var languages: [LanguageOption] = []
    public private(set) var languagesFailed = false
    public var isAgeBlocked: Bool { phase == .blocked }

    public let deps: AppDependencies
    private let defaults: UserDefaults
    private static let cachedUserKey = "cached_user"

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

    private func apply(_ user: UserProfile) {
        self.user = user
        if let lang = user.activeLanguage { activeLanguage = lang }
        if let data = try? JSONEncoder().encode(user) { defaults.set(data, forKey: Self.cachedUserKey) }
    }

    private var cachedUser: UserProfile? {
        defaults.data(forKey: Self.cachedUserKey).flatMap { try? JSONDecoder().decode(UserProfile.self, from: $0) }
    }
}
