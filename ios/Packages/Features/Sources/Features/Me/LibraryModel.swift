import Core
import Foundation
import Observation

/// One Me list (Saved or Liked): newest first, pages on at the end.
@MainActor
@Observable
final class LibraryModel {
    enum State: Equatable { case loading, ready, failed }

    let kind: LibraryKind
    private(set) var cards: [VideoCard] = []
    private(set) var state: State = .loading
    private(set) var cursor: String?
    private var hasMore = true
    private var fetching = false
    private let deps: AppDependencies

    init(kind: LibraryKind, deps: AppDependencies) {
        self.kind = kind
        self.deps = deps
    }

    /// From the top (on appear, so a clip saved in the feed shows first: QA V7-01).
    func reload() async {
        if cards.isEmpty { state = .loading }
        await fetch(cursor: nil, replacing: true)
    }

    func loadMoreIfNeeded(after card: VideoCard) async {
        guard hasMore, !fetching, card.id == cards.last?.id else { return }
        await fetch(cursor: cursor, replacing: false)
    }

    private func fetch(cursor: String?, replacing: Bool) async {
        guard !fetching else { return }
        fetching = true
        defer { fetching = false }
        let feed = deps.feed, kind = kind
        do {
            let page = try await deps.session.authorized { try await feed.library(kind, cursor: cursor, token: $0) }
            if replacing {
                cards = page.items
            } else {
                let known = Set(cards.map(\.id))
                cards += page.items.filter { !known.contains($0.id) }
            }
            self.cursor = page.nextCursor
            hasMore = page.nextCursor != nil
            state = .ready
        } catch {
            if cards.isEmpty { state = .failed }
        }
    }

    /// Unsave or unlike from the context menu: gone at once, back if the call fails.
    func remove(_ card: VideoCard) {
        guard let i = cards.firstIndex(of: card) else { return }
        cards.remove(at: i)
        let feed = deps.feed, kind = kind, id = card.id
        Task {
            do {
                try await deps.session.authorized { token in
                    switch kind {
                    case .saved: try await feed.setSaved(false, videoID: id, token: token)
                    case .liked: try await feed.setLiked(false, videoID: id, token: token)
                    }
                }
            } catch {
                cards.insert(card, at: min(i, cards.count))
            }
        }
    }
}
