import Foundation

/// App-side models. The live adapter maps the generated API types onto these,
/// so screens and tests never depend on generated code.

public struct LearningLanguage: Sendable, Equatable, Codable, Hashable {
    public var language: String
    public var level: Int
    public var isActive: Bool

    public init(language: String, level: Int, isActive: Bool) {
        self.language = language
        self.level = level
        self.isActive = isActive
    }
}

public struct UserProfile: Sendable, Equatable, Codable {
    public var id: String
    public var ageBand: String
    public var uiLanguage: String
    public var nativeLanguage: String
    public var learning: [LearningLanguage]

    public init(id: String, ageBand: String, uiLanguage: String, nativeLanguage: String, learning: [LearningLanguage]) {
        self.id = id
        self.ageBand = ageBand
        self.uiLanguage = uiLanguage
        self.nativeLanguage = nativeLanguage
        self.learning = learning
    }

    /// The language the feed shows: the active one, else the first.
    public var activeLanguage: String? {
        (learning.first(where: \.isActive) ?? learning.first)?.language
    }
}

public struct LanguageOption: Sendable, Equatable, Identifiable, Hashable {
    public var code: String
    public var englishName: String
    public var nativeName: String
    public var isLearnable: Bool
    public var isNativeOption: Bool
    public var launchOrder: Int
    public var id: String { code }

    public init(code: String, englishName: String, nativeName: String, isLearnable: Bool, isNativeOption: Bool, launchOrder: Int) {
        self.code = code
        self.englishName = englishName
        self.nativeName = nativeName
        self.isLearnable = isLearnable
        self.isNativeOption = isNativeOption
        self.launchOrder = launchOrder
    }

    /// Name in the user's interface language, falling back to the English name.
    public func localizedName(locale: Locale = .current) -> String {
        locale.localizedString(forLanguageCode: code)?.capitalized(with: locale) ?? englishName
    }
}

public struct AppConfig: Sendable, Equatable {
    public var feedPageSize: Int
    public var youtubeCaptionsDefault: Bool

    public init(feedPageSize: Int, youtubeCaptionsDefault: Bool) {
        self.feedPageSize = feedPageSize
        self.youtubeCaptionsDefault = youtubeCaptionsDefault
    }

    /// Used until `GET /v1/config` answers, or when it fails.
    public static let fallback = AppConfig(feedPageSize: 10, youtubeCaptionsDefault: true)
}

/// The four onboarding answers. Level is 1, 3 or 5.
public struct OnboardingAnswers: Sendable, Equatable, Codable {
    public var birthYear: Int
    public var learning: String
    public var native: String
    public var level: Int

    public init(birthYear: Int, learning: String, native: String, level: Int) {
        self.birthYear = birthYear
        self.learning = learning
        self.native = native
        self.level = level
    }
}

/// Errors the app reacts to. The live adapter maps HTTP responses onto these.
public enum APIError: Error, Equatable, Sendable {
    /// 401: token missing or expired.
    case unauthorized
    /// 403 age_blocked on registration.
    case ageBlocked
    /// Any other error body: status and the snake_case code.
    case server(status: Int, code: String)
    /// No connection or a transport failure.
    case offline
}

/// CEFR labels for the 1 to 6 level scale, with the plain words the UX uses.
public enum Level {
    public static func cefr(_ level: Int) -> String {
        ["A1", "A2", "B1", "B2", "C1", "C2"][max(1, min(6, level)) - 1]
    }

    public static func plain(_ level: Int) -> String {
        ["Just starting", "Know the basics", "Can get by", "Getting confident", "Pretty comfortable", "Near native"][max(1, min(6, level)) - 1]
    }
}
