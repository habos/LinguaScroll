import Foundation

/// What `POST /v1/sessions/anonymous` needs. Kept in the Keychain so a 401 can
/// re-register the same install without asking the user again.
public struct SessionRequest: Sendable, Equatable, Codable {
    public var installID: String
    public var birthYear: Int
    public var uiLanguage: String
    public var nativeLanguage: String
    public var learning: [LearningLanguage]

    public init(installID: String, birthYear: Int, uiLanguage: String, nativeLanguage: String, learning: [LearningLanguage]) {
        self.installID = installID
        self.birthYear = birthYear
        self.uiLanguage = uiLanguage
        self.nativeLanguage = nativeLanguage
        self.learning = learning
    }
}

public struct SessionResult: Sendable, Equatable {
    public var token: String
    public var user: UserProfile

    public init(token: String, user: UserProfile) {
        self.token = token
        self.user = user
    }
}

/// The one unauthenticated call that creates (or re-finds) this install's user.
public protocol SessionAPI: Sendable {
    func createSession(_ request: SessionRequest) async throws -> SessionResult
}

/// Spec interface (V5): registers the onboarding answers.
public protocol SessionService: Sendable {
    func register(_ answers: OnboardingAnswers) async throws -> UserProfile
}

/// Owns the session token. Registers, stores the token in the Keychain, and on a
/// 401 re-registers with the same install ID (the server returns the same user)
/// and retries the call once.
public actor SessionManager: SessionService {
    private let api: any SessionAPI
    private let keychain: any KeychainStore
    private let uiLanguage: String

    public init(api: any SessionAPI, keychain: any KeychainStore, uiLanguage: String) {
        self.api = api
        self.keychain = keychain
        self.uiLanguage = uiLanguage
    }

    public var token: String? { keychain.string(for: KeychainKey.sessionToken) }

    /// True once this install has registered (survives reinstalls via the Keychain).
    public var isRegistered: Bool { token != nil && storedRequest != nil }

    public func register(_ answers: OnboardingAnswers) async throws -> UserProfile {
        let request = SessionRequest(
            installID: InstallID.current(in: keychain),
            birthYear: answers.birthYear,
            uiLanguage: uiLanguage,
            nativeLanguage: answers.native,
            learning: [LearningLanguage(language: answers.learning, level: answers.level, isActive: true)]
        )
        return try await register(request)
    }

    private func register(_ request: SessionRequest) async throws -> UserProfile {
        do {
            let result = try await api.createSession(request)
            keychain.setString(result.token, for: KeychainKey.sessionToken)
            keychain.set(try JSONEncoder().encode(request), for: KeychainKey.sessionRequest)
            return result.user
        } catch APIError.ageBlocked {
            AgeBlock.set(in: keychain)
            throw APIError.ageBlocked
        }
    }

    /// Runs an authenticated call. On `.unauthorized`, re-registers once and retries once.
    public func authorized<T: Sendable>(_ call: @Sendable (String) async throws -> T) async throws -> T {
        guard let token else { throw APIError.unauthorized }
        do {
            return try await call(token)
        } catch APIError.unauthorized {
            guard let request = storedRequest else { throw APIError.unauthorized }
            _ = try await register(request)
            guard let fresh = self.token else { throw APIError.unauthorized }
            return try await call(fresh)
        }
    }

    /// Reset my data: forget the token, the stored answers and the install ID.
    /// The age block is deliberately kept.
    public func reset() {
        keychain.set(nil, for: KeychainKey.sessionToken)
        keychain.set(nil, for: KeychainKey.sessionRequest)
        keychain.set(nil, for: KeychainKey.installID)
    }

    private var storedRequest: SessionRequest? {
        keychain.data(for: KeychainKey.sessionRequest).flatMap { try? JSONDecoder().decode(SessionRequest.self, from: $0) }
    }
}

/// Unauthenticated catalog calls made on launch.
public protocol CatalogAPI: Sendable {
    func config() async throws -> AppConfig
    func languages() async throws -> [LanguageOption]
}

/// Authenticated calls about this device's user. `token` comes from `SessionManager.authorized`.
public protocol UserAPI: Sendable {
    func me(token: String) async throws -> UserProfile
    /// `PUT /v1/me/languages/{code}`: add a language, change its level, or make it active.
    func putLanguage(_ code: String, level: Int, active: Bool?, token: String) async throws -> UserProfile
}
