import UIKit
import WebKit

/// Collection view whose pan can run alongside the web view's own recognisers.
/// UIScrollView is the delegate of its panGestureRecognizer, so implementing
/// this delegate method on the subclass is enough.
final class FeedCollectionView: UICollectionView {
    var allowSimultaneousPan = true

    // UIScrollView conforms to UIGestureRecognizerDelegate but doesn't implement
    // this optional method, so it's added here (not overridden).
    @objc func gestureRecognizer(
        _ g: UIGestureRecognizer,
        shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer
    ) -> Bool {
        if allowSimultaneousPan && g === panGestureRecognizer { return true }
        return false
    }
}

/// What the spike reports back to the SwiftUI HUD.
@MainActor
final class SpikeStats: ObservableObject {
    @Published var index = 0
    @Published var swipes = 0
    @Published var state = "-"
    @Published var lastTTFF: Int?
    @Published var ttffs: [Int] = []
    @Published var doubleAudioEvents = 0
    @Published var lastEvent = ""

    var medianTTFF: Int? {
        guard !ttffs.isEmpty else { return nil }
        let s = ttffs.sorted()
        return s[s.count / 2]
    }
}

final class FeedViewController: UIViewController, UICollectionViewDataSource, UICollectionViewDelegate {
    private let ids: [String]
    private let stats: SpikeStats
    private let simultaneous: Bool
    private var collectionView: FeedCollectionView!

    /// Exactly two players, ever: slot index % 2.
    private let players = [YouTubePlayer(name: "A"), YouTubePlayer(name: "B")]
    /// Which feed index each slot is currently assigned to.
    private var assigned: [Int?] = [nil, nil]

    private var current = 0
    private var becameCurrentAt = Date()
    private var ttffRecorded = false
    private var nextCued = false
    private var loops = 0
    private var observers: [NSObjectProtocol] = []

    init(ids: [String], simultaneous: Bool, stats: SpikeStats) {
        self.ids = ids
        self.simultaneous = simultaneous
        self.stats = stats
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black

        let layout = UICollectionViewFlowLayout()
        layout.scrollDirection = .vertical
        layout.minimumLineSpacing = 0
        layout.minimumInteritemSpacing = 0
        collectionView = FeedCollectionView(frame: view.bounds, collectionViewLayout: layout)
        collectionView.allowSimultaneousPan = simultaneous
        collectionView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        collectionView.isPagingEnabled = true
        collectionView.showsVerticalScrollIndicator = false
        collectionView.contentInsetAdjustmentBehavior = .never
        collectionView.backgroundColor = .black
        collectionView.dataSource = self
        collectionView.delegate = self
        collectionView.register(FeedCell.self, forCellWithReuseIdentifier: "cell")
        view.addSubview(collectionView)

        for p in players {
            p.onState = { [weak self] player, id, state, pos in
                self?.handle(player: player, videoID: id, state: state, positionMs: pos)
            }
        }

        let nc = NotificationCenter.default
        observers.append(nc.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.players.forEach { $0.pause() } }
        })
        observers.append(nc.addObserver(forName: UIApplication.willEnterForegroundNotification, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.playCurrent() }
        })
    }

    deinit { observers.forEach(NotificationCenter.default.removeObserver) }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        if let layout = collectionView.collectionViewLayout as? UICollectionViewFlowLayout,
           layout.itemSize != collectionView.bounds.size, collectionView.bounds.height > 0 {
            layout.itemSize = collectionView.bounds.size
            layout.invalidateLayout()
        }
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        becomeCurrent(0)
    }

    // MARK: Data source

    func collectionView(_ cv: UICollectionView, numberOfItemsInSection section: Int) -> Int { ids.count }

    func collectionView(_ cv: UICollectionView, cellForItemAt indexPath: IndexPath) -> UICollectionViewCell {
        let cell = cv.dequeueReusableCell(withReuseIdentifier: "cell", for: indexPath) as! FeedCell
        cell.configure(videoID: ids[indexPath.item], index: indexPath.item)
        // A reused cell must not keep showing a player that now belongs elsewhere.
        if let hosted = cell.hostedWebView,
           let slot = players.firstIndex(where: { $0.webView === hosted }),
           assigned[slot] != indexPath.item {
            hosted.removeFromSuperview()
        }
        return cell
    }

    func collectionView(_ cv: UICollectionView, willDisplay cell: UICollectionViewCell, forItemAt indexPath: IndexPath) {
        attachPlayer(to: cell as! FeedCell, index: indexPath.item)
    }

    // MARK: Paging

    func scrollViewDidScroll(_ sv: UIScrollView) {
        let h = sv.bounds.height
        guard h > 0 else { return }
        // Pause as soon as the current clip is no longer more than half visible.
        let visible = 1 - abs(sv.contentOffset.y - CGFloat(current) * h) / h
        if visible <= 0.5 { players[current % 2].pause() }
    }

    func scrollViewDidEndDecelerating(_ sv: UIScrollView) { settle(sv) }

    func scrollViewDidEndDragging(_ sv: UIScrollView, willDecelerate decelerate: Bool) {
        if !decelerate { settle(sv) }
    }

    func scrollViewDidEndScrollingAnimation(_ sv: UIScrollView) { settle(sv) }

    private func settle(_ sv: UIScrollView) {
        let h = sv.bounds.height
        guard h > 0 else { return }
        let index = max(0, min(ids.count - 1, Int((sv.contentOffset.y / h).rounded())))
        if index != current {
            stats.swipes += 1
            becomeCurrent(index)
        } else {
            playCurrent() // dragged and let go on the same clip
        }
    }

    private func becomeCurrent(_ index: Int) {
        guard !ids.isEmpty else { return }
        if index != current {
            stats.lastEvent = "view_end #\(current) loops=\(loops)"
        }
        current = index
        becameCurrentAt = Date()
        ttffRecorded = false
        nextCued = false
        loops = 0
        stats.index = index

        // Slot for this clip plays it; the other slot gets the next clip.
        let mine = index % 2
        let other = 1 - mine
        players[other].pause()
        assigned[mine] = index
        assigned[other] = index + 1 < ids.count ? index + 1 : nil

        for case let cell as FeedCell in collectionView.visibleCells {
            if let ip = collectionView.indexPath(for: cell) { attachPlayer(to: cell, index: ip.item) }
        }
        playCurrent()
    }

    private func playCurrent() {
        guard current < ids.count else { return }
        players[current % 2].play(ids[current])
    }

    private func attachPlayer(to cell: FeedCell, index: Int) {
        for (slot, idx) in assigned.enumerated() where idx == index {
            cell.host(players[slot].webView)
            if index != current { players[slot].cue(ids[index]) }
        }
    }

    // MARK: Player events

    private func handle(player: YouTubePlayer, videoID: String?, state: YTState, positionMs: Int) {
        let isCurrent = player === players[current % 2] && videoID == ids[safe: current]

        // Single-audio check: count players reporting `playing`.
        if players.filter({ $0.state == .playing }).count > 1 {
            stats.doubleAudioEvents += 1
        }

        guard isCurrent else { return }
        stats.state = "\(player.name): \(state)"

        switch state {
        case .playing:
            if !ttffRecorded {
                ttffRecorded = true
                let ms = Int(Date().timeIntervalSince(becameCurrentAt) * 1000)
                stats.lastTTFF = ms
                stats.ttffs.append(ms)
            }
            if !nextCued, current + 1 < ids.count {
                nextCued = true
                players[(current + 1) % 2].cue(ids[current + 1])
            }
        case .ended:
            loops += 1
            player.seekToStartAndPlay()
        case .error(let code):
            stats.lastEvent = "player_error \(code) on \(videoID ?? "?")"
            let from = current
            DispatchQueue.main.asyncAfter(deadline: .now() + 1) { [weak self] in
                guard let self, self.current == from, from + 1 < self.ids.count else { return }
                self.collectionView.scrollToItem(at: IndexPath(item: from + 1, section: 0), at: .top, animated: true)
            }
        default:
            break
        }
    }
}

extension Array {
    subscript(safe i: Int) -> Element? { indices.contains(i) ? self[i] : nil }
}
