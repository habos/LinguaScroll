import APIClient
import Core
import Foundation
import OpenAPIRuntime

/// Maps the generated client (api/openapi.yaml) onto Core's protocols.
struct LiveAPI: SessionAPI, CatalogAPI, UserAPI, FeedAPI {
    let baseURL: URL

    private func client(_ token: String? = nil) -> Client {
        LinguaScrollAPI.client(baseURL: baseURL, token: token)
    }

    func createSession(_ r: SessionRequest) async throws -> SessionResult {
        let body = Components.Schemas.AnonymousSessionRequest(
            installId: r.installID,
            birthYear: r.birthYear,
            uiLanguage: r.uiLanguage,
            nativeLanguage: r.nativeLanguage,
            learning: r.learning.map { .init(language: $0.language, level: $0.level) }
        )
        let output = try await transport { try await client().createAnonymousSession(body: .json(body)) }
        switch output {
        case .created(let ok):
            let s = try ok.body.json
            return SessionResult(token: s.token, user: Self.user(s.user))
        case .forbidden(let r):
            let code = (try? r.body.json)?.error.code ?? "forbidden"
            throw code == "age_blocked" ? APIError.ageBlocked : APIError.server(status: 403, code: code)
        case .unprocessableContent(let r):
            throw APIError.server(status: 422, code: (try? r.body.json)?.error.code ?? "unprocessable")
        case .default(let status, let r):
            throw Self.error(status, try? r.body.json)
        }
    }

    func config() async throws -> AppConfig {
        switch try await transport({ try await client().getConfig() }) {
        case .ok(let ok):
            let c = try ok.body.json
            return AppConfig(feedPageSize: c.feedPageSize, youtubeCaptionsDefault: c.youtubeCaptionsDefault)
        case .default(let status, let r):
            throw Self.error(status, try? r.body.json)
        }
    }

    func languages() async throws -> [LanguageOption] {
        switch try await transport({ try await client().listLanguages() }) {
        case .ok(let ok):
            return try ok.body.json.items.map {
                LanguageOption(code: $0.code, englishName: $0.englishName, nativeName: $0.nativeName,
                               isLearnable: $0.isLearnable, isNativeOption: $0.isNativeOption, launchOrder: $0.launchOrder)
            }
        case .default(let status, let r):
            throw Self.error(status, try? r.body.json)
        }
    }

    func me(token: String) async throws -> UserProfile {
        switch try await transport({ try await client(token).getMe() }) {
        case .ok(let ok):
            return Self.user(try ok.body.json)
        case .default(let status, let r):
            throw Self.error(status, try? r.body.json)
        }
    }

    func putLanguage(_ code: String, level: Int, active: Bool?, token: String) async throws -> UserProfile {
        let output = try await transport {
            try await client(token).putMyLanguage(path: .init(code: code), body: .json(.init(level: level, isActive: active)))
        }
        switch output {
        case .ok(let ok): return Self.user(try ok.body.json)
        case .default(let status, let r): throw Self.error(status, try? r.body.json)
        }
    }

    func removeLanguage(_ code: String, token: String) async throws -> UserProfile {
        switch try await transport({ try await client(token).deleteMyLanguage(path: .init(code: code)) }) {
        case .ok(let ok): return Self.user(try ok.body.json)
        case .unprocessableContent(let r): throw APIError.server(status: 422, code: (try? r.body.json)?.error.code ?? "unprocessable")
        case .default(let status, let r): throw Self.error(status, try? r.body.json)
        }
    }

    func setNativeLanguage(_ code: String, token: String) async throws -> UserProfile {
        switch try await transport({ try await client(token).updateMe(body: .json(.init(nativeLanguage: code))) }) {
        case .ok(let ok): return Self.user(try ok.body.json)
        case .default(let status, let r): throw Self.error(status, try? r.body.json)
        }
    }

    func deleteMe(token: String) async throws {
        switch try await transport({ try await client(token).deleteMe() }) {
        case .noContent: return
        case .default(let status, let r): throw Self.error(status, try? r.body.json)
        }
    }

    // MARK: Feed and clip actions

    func library(_ kind: LibraryKind, cursor: String?, token: String) async throws -> FeedPage {
        let c = client(token)
        switch kind {
        case .saved:
            switch try await transport({ try await c.listMySaves(query: .init(cursor: cursor)) }) {
            case .ok(let ok):
                let page = try ok.body.json
                return FeedPage(items: page.items.map(Self.card), nextCursor: page.nextCursor)
            case .default(let status, let r):
                throw Self.error(status, try? r.body.json)
            }
        case .liked:
            switch try await transport({ try await c.listMyLikes(query: .init(cursor: cursor)) }) {
            case .ok(let ok):
                let page = try ok.body.json
                return FeedPage(items: page.items.map(Self.card), nextCursor: page.nextCursor)
            case .default(let status, let r):
                throw Self.error(status, try? r.body.json)
            }
        }
    }

    func feed(language: String, cursor: String?, token: String) async throws -> FeedPage {
        switch try await transport({ try await client(token).getFeed(query: .init(language: language, cursor: cursor)) }) {
        case .ok(let ok):
            let page = try ok.body.json
            return FeedPage(items: page.items.map(Self.card), nextCursor: page.nextCursor)
        case .default(let status, let r):
            throw Self.error(status, try? r.body.json)
        }
    }

    func setLiked(_ liked: Bool, videoID: String, token: String) async throws {
        let c = client(token)
        if liked {
            switch try await transport({ try await c.likeVideo(path: .init(id: videoID)) }) {
            case .noContent: return
            case .default(let status, let r): throw Self.error(status, try? r.body.json)
            }
        } else {
            switch try await transport({ try await c.unlikeVideo(path: .init(id: videoID)) }) {
            case .noContent: return
            case .default(let status, let r): throw Self.error(status, try? r.body.json)
            }
        }
    }

    func setSaved(_ saved: Bool, videoID: String, token: String) async throws {
        let c = client(token)
        if saved {
            switch try await transport({ try await c.saveVideo(path: .init(id: videoID)) }) {
            case .noContent: return
            case .default(let status, let r): throw Self.error(status, try? r.body.json)
            }
        } else {
            switch try await transport({ try await c.unsaveVideo(path: .init(id: videoID)) }) {
            case .noContent: return
            case .default(let status, let r): throw Self.error(status, try? r.body.json)
            }
        }
    }

    func hide(videoID: String, token: String) async throws {
        switch try await transport({ try await client(token).hideVideo(path: .init(id: videoID)) }) {
        case .noContent: return
        case .default(let status, let r): throw Self.error(status, try? r.body.json)
        }
    }

    func setChannelBlocked(_ blocked: Bool, channelID: String, token: String) async throws {
        let c = client(token)
        if blocked {
            switch try await transport({ try await c.blockChannel(path: .init(channelId: channelID)) }) {
            case .noContent: return
            case .default(let status, let r): throw Self.error(status, try? r.body.json)
            }
        } else {
            switch try await transport({ try await c.unblockChannel(path: .init(channelId: channelID)) }) {
            case .noContent: return
            case .default(let status, let r): throw Self.error(status, try? r.body.json)
            }
        }
    }

    func report(videoID: String, reason: ReportReason, token: String) async throws {
        let body = Components.Schemas.ReportRequest(reason: .init(rawValue: reason.rawValue)!)
        switch try await transport({ try await client(token).reportVideo(path: .init(id: videoID), body: .json(body)) }) {
        case .noContent: return
        case .default(let status, let r): throw Self.error(status, try? r.body.json)
        }
    }

    func postEvents(_ events: [ClientEvent], token: String) async throws {
        let payload = try events.map { e in
            Components.Schemas.Event(
                id: e.id.uuidString.lowercased(),
                _type: .init(rawValue: e.type.rawValue)!,
                at: e.at,
                videoId: e.videoID,
                props: .init(additionalProperties: try OpenAPIObjectContainer(unvalidatedValue: e.props.mapValues(Self.any)))
            )
        }
        switch try await transport({ try await client(token).postEvents(body: .json(.init(events: payload))) }) {
        case .accepted:
            return
        case .default(let status, let r):
            throw Self.error(status, try? r.body.json)
        }
    }

    // MARK: Mapping

    private static func user(_ u: Components.Schemas.User) -> UserProfile {
        UserProfile(
            id: u.id,
            ageBand: u.ageBand.rawValue,
            uiLanguage: u.uiLanguage,
            nativeLanguage: u.nativeLanguage,
            learning: u.learning.map { LearningLanguage(language: $0.language, level: $0.level, isActive: $0.isActive) }
        )
    }

    private static func card(_ v: Components.Schemas.VideoCard) -> VideoCard {
        VideoCard(
            id: v.id, language: v.language, level: v.level, durationMs: v.durationMs, aspect: v.aspect,
            youtubeID: v.youtube.videoId, title: v.youtube.title, channelID: v.youtube.channelId,
            channelTitle: v.youtube.channelTitle, hasCaptions: v.youtube.hasCaptions,
            liked: v.viewer.liked, saved: v.viewer.saved
        )
    }

    private static func error(_ status: Int, _ body: Components.Schemas._Error?) -> APIError {
        if status == 401 { return .unauthorized }
        return .server(status: status, code: body?.error.code ?? "http_\(status)")
    }

    private static func any(_ v: JSONValue) -> (any Sendable)? {
        switch v {
        case .int(let i): i
        case .double(let d): d
        case .bool(let b): b
        case .string(let s): s
        }
    }

    /// Anything thrown that isn't already an APIError (no network, timeouts) counts as offline.
    private func transport<T>(_ call: () async throws -> T) async throws -> T {
        do {
            return try await call()
        } catch let e as APIError {
            throw e
        } catch {
            throw APIError.offline
        }
    }
}

/// Sends event batches with the session token; a 401 re-registers once (SessionManager).
struct LiveEventSink: EventSink {
    let api: LiveAPI
    let session: SessionManager

    func send(_ events: [ClientEvent]) async throws {
        try await session.authorized { try await api.postEvents(events, token: $0) }
    }
}
