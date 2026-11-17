import Foundation

/// One clip in the feed (`VideoCard` in the API).
public struct VideoCard: Sendable, Equatable, Identifiable, Hashable {
    public var id: String            // our video UUID (used in API paths and events)
    public var language: String
    public var level: Int?
    public var durationMs: Int
    public var aspect: Double        // width / height
    public var youtubeID: String
    public var title: String
    public var channelID: String
    public var channelTitle: String
    public var hasCaptions: Bool
    public var liked: Bool
    public var saved: Bool

    public init(id: String, language: String, level: Int?, durationMs: Int, aspect: Double, youtubeID: String,
                title: String, channelID: String, channelTitle: String, hasCaptions: Bool, liked: Bool, saved: Bool) {
        self.id = id
        self.language = language
        self.level = level
        self.durationMs = durationMs
        self.aspect = aspect
        self.youtubeID = youtubeID
        self.title = title
        self.channelID = channelID
        self.channelTitle = channelTitle
        self.hasCaptions = hasCaptions
        self.liked = liked
        self.saved = saved
    }

    public var shareURL: URL { URL(string: "https://youtube.com/shorts/\(youtubeID)")! }
    public var channelURL: URL { URL(string: "https://www.youtube.com/channel/\(channelID)")! }
    public var thumbnailURL: URL { URL(string: "https://i.ytimg.com/vi/\(youtubeID)/hqdefault.jpg")! }
}

public struct FeedPage: Sendable, Equatable {
    /// Why an empty, final feed page is empty.
    public enum EndReason: String, Sendable, Equatable {
        /// The language has clips, but this device has been served all of them for now.
        case caughtUp = "caught_up"
        /// There are no clips in this language yet.
        case noClips = "no_clips"
    }

    public var items: [VideoCard]
    public var nextCursor: String?
    public var endReason: EndReason?

    public init(items: [VideoCard], nextCursor: String?, endReason: EndReason? = nil) {
        self.items = items
        self.nextCursor = nextCursor
        self.endReason = endReason
    }
}

/// The two lists on the Me screen.
public enum LibraryKind: String, Sendable, CaseIterable, Hashable {
    case saved, liked
}

public enum ReportReason: String, Sendable, CaseIterable {
    case notThisLanguage = "not_this_language"
    case inappropriate
    case spam
    case other
}

/// Authenticated feed and clip actions. Tokens come from `SessionManager.authorized`.
public protocol FeedAPI: Sendable {
    /// Empty `items` with nil `nextCursor` = nothing left to serve; `endReason` says whether the device
    /// has watched everything (`caughtUp`) or the language has no clips yet (`noClips`).
    /// Throws `.server(400, "bad_cursor")` for a stale cursor: drop it and refetch.
    func feed(language: String, cursor: String?, token: String) async throws -> FeedPage
    func setLiked(_ liked: Bool, videoID: String, token: String) async throws
    func setSaved(_ saved: Bool, videoID: String, token: String) async throws
    func hide(videoID: String, token: String) async throws
    func setChannelBlocked(_ blocked: Bool, channelID: String, token: String) async throws
    func report(videoID: String, reason: ReportReason, token: String) async throws
    /// `GET /v1/me/saves` or `/v1/me/likes`: newest first, same paging as the feed.
    func library(_ kind: LibraryKind, cursor: String?, token: String) async throws -> FeedPage
}
