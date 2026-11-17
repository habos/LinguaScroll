import Core
import Foundation
import Observation

/// A short line that replaces the channel column in the action bar for 2.5 s.
public struct BarMessage: Equatable, Sendable {
    public enum Kind: Equatable, Sendable { case firstLike, actionFailed, playerError, hidden }
    public let id = UUID()
    public let kind: Kind

    public var line1: String {
        switch kind {
        case .firstLike: String(localized: "Liked")
        case .actionFailed: String(localized: "Couldn't save that")
        case .playerError: String(localized: "This clip can't play here")
        case .hidden: String(localized: "Hidden")
        }
    }

    public var line2: String {
        switch kind {
        case .firstLike: String(localized: "Likes shape what you see next")
        case .actionFailed: String(localized: "Check your connection and try again")
        case .playerError: String(localized: "Skipping to the next one…")
        case .hidden: String(localized: "Undo")
        }
    }
}

/// Feed state for one language: paging, optimistic like/save, hide/block with undo, and events.
@MainActor
@Observable
public final class FeedModel {
    public enum LoadState: Equatable { case loading, ready, offline, caughtUp }

    public let language: String
    public private(set) var cards: [VideoCard] = []
    public private(set) var state: LoadState = .loading
    public private(set) var barMessage: BarMessage?
    public private(set) var currentIndex = 0
    public var captionsOn: Bool
    /// YouTube's own progress bar and title on the player. Off by default (Harry, 2026-11-12):
    /// `controls=0` is a supported IFrame parameter, nothing of ours covers the player.
    /// Debug switch: `defaults write app.linguascroll.ios feed.youtubeControls -bool YES`.
    public var youtubeControls: Bool { defaults.bool(forKey: Self.youtubeControlsKey) }
    static let youtubeControlsKey = "feed.youtubeControls"
    /// True while SwiftUI shows something over the feed (e.g. the language switcher): the clip pauses.
    public var externallyPaused = false

    /// Called when `cards` changes shape (insert/remove), so the collection view can apply a snapshot.
    @ObservationIgnored public var onCardsChanged: (() -> Void)?
    /// Called to move the feed (after hide/block/error).
    @ObservationIgnored public var onScrollTo: ((Int) -> Void)?

    private let deps: AppDependencies
    private let defaults: UserDefaults
    private var cursor: String?
    private var hasMore = true
    private var fetching = false
    /// The last hide or block, for Undo: removed cards, the server call, and how to reverse it.
    private var lastRemoval: (cards: [(Int, VideoCard)], sent: Task<Void, Never>, revert: @Sendable (String) async throws -> Void)?
    private var messageTask: Task<Void, Never>?

    static let prefetchWhenRemaining = 5
    static let firstLikeKey = "seen_first_like_hint"

    /// Where pages come from: the ranked feed, or a Me list played from one of its clips.
    public enum Source: Equatable, Sendable { case feed, library(LibraryKind) }
    public let source: Source

    public init(language: String, deps: AppDependencies, captionsOn: Bool, defaults: UserDefaults = .standard) {
        self.language = language
        self.source = .feed
        self.deps = deps
        self.captionsOn = captionsOn
        self.defaults = defaults
    }

    /// Plays a Saved or Liked list from `startIndex`, in its order, paging on with `cursor`.
    public init(library kind: LibraryKind, cards: [VideoCard], cursor: String?, startIndex: Int,
                deps: AppDependencies, captionsOn: Bool, defaults: UserDefaults = .standard) {
        let start = min(max(0, startIndex), max(0, cards.count - 1))
        self.language = cards[safe: start]?.language ?? ""
        self.source = .library(kind)
        self.deps = deps
        self.captionsOn = captionsOn
        self.defaults = defaults
        self.cards = cards
        self.cursor = cursor
        self.hasMore = cursor != nil
        self.currentIndex = start
        self.state = cards.isEmpty ? .caughtUp : .ready
    }

    public var current: VideoCard? { cards.indices.contains(currentIndex) ? cards[currentIndex] : nil }

    // MARK: Paging

    public func start() async {
        guard cards.isEmpty else { return }
        state = .loading
        cursor = nil
        hasMore = true
        await loadMore()
    }

    /// Retry after offline or caught-up (e.g. on foreground or the Try again button).
    public func retry() async {
        if state == .caughtUp { hasMore = true; cursor = nil }
        if cards.isEmpty { state = .loading }
        await loadMore()
    }

    public func loadMore() async {
        guard !fetching, hasMore else { return }
        fetching = true
        defer { fetching = false }
        let feed = deps.feed, language = language, cursor = cursor
        do {
            let page: FeedPage
            switch source {
            case .feed:
                page = try await deps.session.authorized { try await feed.feed(language: language, cursor: cursor, token: $0) }
            case .library(let kind):
                page = try await deps.session.authorized { try await feed.library(kind, cursor: cursor, token: $0) }
            }
            let known = Set(cards.map(\.id))
            let fresh = page.items.filter { !known.contains($0.id) }
            if !fresh.isEmpty {
                cards.append(contentsOf: fresh)
                onCardsChanged?()
            }
            self.cursor = page.nextCursor
            hasMore = page.nextCursor != nil
            // Empty page + no cursor = watched everything for now. With clips still loaded,
            // keep playing them; `retry()` (on foreground) asks again from the top.
            state = cards.isEmpty && !hasMore ? .caughtUp : .ready
        } catch APIError.server(400, "bad_cursor") {
            self.cursor = nil
            fetching = false
            await loadMore()
        } catch {
            if cards.isEmpty { state = .offline }
        }
    }

    // MARK: Current clip

    public func becameCurrent(_ index: Int) {
        guard cards.indices.contains(index) else { return }
        currentIndex = index
        deps.events.log(.impression, videoID: cards[index].id, props: ["position": .int(index)])
        if cards.count - index <= Self.prefetchWhenRemaining {
            Task { await loadMore() }
        }
    }

    public func viewEnded(_ card: VideoCard, watchedMs: Int, loops: Int, ttffMs: Int?) {
        let completed = loops > 0 || watchedMs >= card.durationMs - 500
        var props: [String: JSONValue] = [
            "watched_ms": .int(watchedMs),
            "duration_ms": .int(card.durationMs),
            "loops": .int(loops),
            "completed": .bool(completed),
        ]
        if let ttffMs { props["ttff_ms"] = .int(ttffMs) }
        deps.events.log(.viewEnd, videoID: card.id, props: props)
    }

    public func shared(_ card: VideoCard) {
        deps.events.log(.share, videoID: card.id, props: [:])
    }

    /// Logs, shows the bar message, and advances after 1 s. 101/150 (embedding disabled) also hide the clip.
    public func playerError(_ card: VideoCard, code: Int) {
        deps.events.log(.playerError, videoID: card.id, props: ["code": .int(code)])
        show(.playerError)
        if code == 101 || code == 150 {
            let feed = deps.feed, id = card.id
            Task { _ = try? await deps.session.authorized { try await feed.hide(videoID: id, token: $0) } }
        }
        Task {
            try? await Task.sleep(for: .seconds(1))
            guard let i = cards.firstIndex(of: card), i == currentIndex, i + 1 < cards.count else { return }
            onScrollTo?(i + 1)
        }
    }

    // MARK: Like and save (optimistic, rolled back on failure)

    public func toggleLike(_ id: String) {
        guard let i = cards.firstIndex(where: { $0.id == id }) else { return }
        let new = !cards[i].liked
        cards[i].liked = new
        if new && !defaults.bool(forKey: Self.firstLikeKey) {
            defaults.set(true, forKey: Self.firstLikeKey)
            show(.firstLike)
        }
        let feed = deps.feed
        Task {
            do {
                try await deps.session.authorized { try await feed.setLiked(new, videoID: id, token: $0) }
            } catch APIError.server(404, _) {
                removeNow { $0.id == id } // the clip was removed from the catalog: already gone, no error
            } catch {
                if let j = cards.firstIndex(where: { $0.id == id }) { cards[j].liked = !new }
                show(.actionFailed)
            }
        }
    }

    public func toggleSave(_ id: String) {
        guard let i = cards.firstIndex(where: { $0.id == id }) else { return }
        let new = !cards[i].saved
        cards[i].saved = new
        let feed = deps.feed
        Task {
            do {
                try await deps.session.authorized { try await feed.setSaved(new, videoID: id, token: $0) }
            } catch APIError.server(404, _) {
                removeNow { $0.id == id } // the clip was removed from the catalog: already gone, no error
            } catch {
                if let j = cards.firstIndex(where: { $0.id == id }) { cards[j].saved = !new }
                show(.actionFailed)
            }
        }
    }

    // MARK: Hide, block, report

    /// Removes the clip and hides it on the server now; Undo (while the bar shows "Hidden")
    /// calls `DELETE /v1/videos/{id}/hide`.
    public func hide(_ card: VideoCard) {
        let feed = deps.feed, id = card.id
        remove({ $0.id == id },
               commit: { try await feed.hide(videoID: id, token: $0) },
               revert: { try await feed.unhide(videoID: id, token: $0) })
    }

    /// Removes this channel's loaded clips (current and later) and blocks it now; Undo unblocks.
    public func blockChannel(_ card: VideoCard) {
        let feed = deps.feed, channel = card.channelID, from = currentIndex
        var index = -1
        remove({ c in index += 1; return c.channelID == channel && index >= from },
               commit: { try await feed.setChannelBlocked(true, channelID: channel, token: $0) },
               revert: { try await feed.setChannelBlocked(false, channelID: channel, token: $0) })
    }

    /// Reports and hides for this device (no undo for reports).
    public func report(_ card: VideoCard, reason: ReportReason) {
        let feed = deps.feed, id = card.id
        Task { _ = try? await deps.session.authorized { try await feed.report(videoID: id, reason: reason, token: $0) } }
        removeNow { $0.id == id }
    }

    public func undo() {
        guard let last = lastRemoval else { return }
        lastRemoval = nil
        for (i, card) in last.cards.sorted(by: { $0.0 < $1.0 }) {
            cards.insert(card, at: min(i, cards.count))
        }
        barMessage = nil
        onCardsChanged?()
        if let first = last.cards.map(\.0).min() { onScrollTo?(first) }
        let session = deps.session, revert = last.revert, sent = last.sent
        Task {
            await sent.value // the hide must reach the server before the un-hide
            _ = try? await session.authorized { try await revert($0) }
        }
    }

    private func remove(_ match: (VideoCard) -> Bool,
                        commit: @escaping @Sendable (String) async throws -> Void,
                        revert: @escaping @Sendable (String) async throws -> Void) {
        let removed = removeNow(match)
        guard !removed.isEmpty else { return }
        let session = deps.session
        let sent = Task { _ = try? await session.authorized { try await commit($0) } }
        lastRemoval = (removed, sent, revert)
        show(.hidden)
    }

    @discardableResult
    private func removeNow(_ match: (VideoCard) -> Bool) -> [(Int, VideoCard)] {
        var removed: [(Int, VideoCard)] = []
        var kept: [VideoCard] = []
        for (i, c) in cards.enumerated() {
            if match(c) { removed.append((i, c)) } else { kept.append(c) }
        }
        guard !removed.isEmpty else { return [] }
        cards = kept
        currentIndex = min(currentIndex, max(0, cards.count - 1))
        onCardsChanged?()
        if cards.isEmpty { state = .loading; Task { await loadMore() } }
        else if cards.count - currentIndex <= Self.prefetchWhenRemaining { Task { await loadMore() } }
        return removed
    }

    // MARK: Bar messages

    private func show(_ kind: BarMessage.Kind) {
        let message = BarMessage(kind: kind)
        barMessage = message
        messageTask?.cancel()
        messageTask = Task {
            try? await Task.sleep(for: .seconds(2.5))
            guard !Task.isCancelled, barMessage == message else { return }
            barMessage = nil
            if kind == .hidden { lastRemoval = nil } // the Undo window is the "Hidden" message
        }
    }
}
