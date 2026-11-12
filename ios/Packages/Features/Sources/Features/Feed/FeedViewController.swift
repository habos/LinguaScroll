import Core
import SwiftUI
import UIKit
import WebKit

/// Paging collection view whose pan runs alongside the web view's recognisers, so a
/// vertical swipe that starts on the player pages the feed (the V6 spike tests this).
/// UIScrollView is its pan recogniser's delegate but doesn't implement this method.
final class FeedCollectionView: UICollectionView {
    @objc func gestureRecognizer(_ g: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool {
        g === panGestureRecognizer
    }
}

/// The feed: player area that pages, with the action bar pinned under it.
/// Exactly two YouTube players: slot `index % 2`. The current one plays; the next is cued.
final class FeedViewController: UIViewController, UICollectionViewDelegate {
    let model: FeedModel
    private let openURL: (URL) -> Void
    private var collectionView: FeedCollectionView!
    private var dataSource: UICollectionViewDiffableDataSource<Int, String>!
    private let bar = ActionBar()
    private(set) var players: [YouTubePlayer] = []
    private var assigned: [String?] = [nil, nil] // card id per slot
    private var current = 0

    // Per-view measurements for view_end.
    private var becameCurrentAt = Date()
    private var ttffMs: Int?
    private var playingSince: Date?
    private var watchedMs = 0
    private var loops = 0
    private var currentCardID: String?
    private var sheetOpen = false
    private var swiftUIPaused = false

    init(model: FeedModel, openURL: @escaping (URL) -> Void) {
        self.model = model
        self.openURL = openURL
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        let captions = model.captionsOn ? model.language : nil
        let controls = model.youtubeControls
        players = ["A", "B"].map { YouTubePlayer(name: $0, captionsLanguage: captions, controls: controls) }

        let layout = UICollectionViewFlowLayout()
        layout.minimumLineSpacing = 0
        collectionView = FeedCollectionView(frame: .zero, collectionViewLayout: layout)
        collectionView.isPagingEnabled = true
        collectionView.showsVerticalScrollIndicator = false
        collectionView.contentInsetAdjustmentBehavior = .never
        collectionView.backgroundColor = .black
        collectionView.delegate = self
        collectionView.register(FeedCell.self, forCellWithReuseIdentifier: "cell")
        collectionView.accessibilityIdentifier = "feed"

        dataSource = UICollectionViewDiffableDataSource(collectionView: collectionView) { [weak self] cv, indexPath, id in
            let cell = cv.dequeueReusableCell(withReuseIdentifier: "cell", for: indexPath) as! FeedCell
            guard let self, let card = self.model.cards.first(where: { $0.id == id }) else { return cell }
            if let hosted = cell.hostedWebView, let slot = self.slot(of: hosted), self.assigned[slot] != id {
                cell.releaseWebView()
            }
            cell.configure(card)
            return cell
        }

        bar.translatesAutoresizingMaskIntoConstraints = false
        collectionView.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(collectionView)
        view.addSubview(bar)
        NSLayoutConstraint.activate([
            collectionView.topAnchor.constraint(equalTo: view.topAnchor),
            collectionView.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            collectionView.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            collectionView.bottomAnchor.constraint(equalTo: bar.topAnchor),
            bar.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            bar.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            bar.bottomAnchor.constraint(equalTo: view.bottomAnchor),
        ])
        wireBar()

        for p in players {
            p.onState = { [weak self] player, videoID, state, _ in self?.handle(player, videoID: videoID, state: state) }
        }
        model.onCardsChanged = { [weak self] in self?.applySnapshot() }
        model.onScrollTo = { [weak self] index in
            guard let self, index < self.model.cards.count else { return }
            self.collectionView.scrollToItem(at: IndexPath(item: index, section: 0), at: .top, animated: true)
        }

        // Selector-based observers are removed automatically when the controller goes away.
        let nc = NotificationCenter.default
        nc.addObserver(self, selector: #selector(didEnterBackground), name: UIApplication.didEnterBackgroundNotification, object: nil)
        nc.addObserver(self, selector: #selector(willEnterForeground), name: UIApplication.willEnterForegroundNotification, object: nil)
        applySnapshot()
        startObservingModel()
    }

    @objc private func didEnterBackground() { pauseCurrent(endView: true) }
    @objc private func willEnterForeground() { becomeCurrent(current, force: true) }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        if let layout = collectionView.collectionViewLayout as? UICollectionViewFlowLayout,
           collectionView.bounds.height > 0, layout.itemSize != collectionView.bounds.size {
            layout.itemSize = collectionView.bounds.size
            layout.invalidateLayout()
        }
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        if !sheetOpen { becomeCurrent(current, force: true) }
    }

    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        if !sheetOpen { pauseCurrent(endView: true) }
    }

    // MARK: Model → UI

    private func startObservingModel() {
        withObservationTracking {
            _ = model.barMessage
            _ = model.cards
            _ = model.externallyPaused
        } onChange: { [weak self] in
            Task { @MainActor in
                guard let self else { return }
                if self.model.externallyPaused != self.swiftUIPaused {
                    self.swiftUIPaused = self.model.externallyPaused
                    if self.swiftUIPaused {
                        self.pauseCurrent(endView: false)
                        self.sheetOpen = true
                    } else {
                        self.sheetClosed()
                    }
                }
                self.bar.show(self.model.barMessage)
                self.bar.configure(self.model.cards[safe: self.current])
                self.startObservingModel()
            }
        }
    }

    private func applySnapshot() {
        var snap = NSDiffableDataSourceSnapshot<Int, String>()
        snap.appendSections([0])
        snap.appendItems(model.cards.map(\.id))
        let wasEmpty = dataSource.snapshot().numberOfItems == 0
        dataSource.apply(snap, animatingDifferences: !wasEmpty) { [weak self] in
            guard let self else { return }
            // Removing the current clip moves the next one into place.
            let index = min(self.model.currentIndex, max(0, self.model.cards.count - 1))
            if wasEmpty || self.model.cards[safe: index]?.id != self.currentCardID {
                self.becomeCurrent(index, force: true)
            }
        }
        bar.configure(model.cards[safe: current])
    }

    // MARK: Paging

    func scrollViewDidScroll(_ sv: UIScrollView) {
        let h = sv.bounds.height
        guard h > 0 else { return }
        let visible = 1 - abs(sv.contentOffset.y - CGFloat(current) * h) / h
        if visible <= 0.5 { pauseCurrent(endView: false) }
        // Start the next clip as soon as it is more than half visible (the earliest YouTube
        // allows), not when the swipe settles. Only for user swipes; settle() covers the rest.
        guard sv.isTracking || sv.isDragging || sv.isDecelerating, currentCardID != nil else { return }
        let nearest = max(0, min(model.cards.count - 1, Int((sv.contentOffset.y / h).rounded())))
        let nearestVisible = 1 - abs(sv.contentOffset.y - CGFloat(nearest) * h) / h
        if nearestVisible > 0.5, let card = model.cards[safe: nearest], card.id != currentCardID {
            becomeCurrent(nearest, force: false)
        }
    }

    func scrollViewDidEndDecelerating(_ sv: UIScrollView) { settle() }
    func scrollViewDidEndDragging(_ sv: UIScrollView, willDecelerate d: Bool) { if !d { settle() } }
    func scrollViewDidEndScrollingAnimation(_ sv: UIScrollView) { settle() }

    func collectionView(_ cv: UICollectionView, willDisplay cell: UICollectionViewCell, forItemAt ip: IndexPath) {
        attach(to: cell as! FeedCell, index: ip.item)
    }

    private func settle() {
        let h = collectionView.bounds.height
        guard h > 0, !model.cards.isEmpty else { return }
        let index = max(0, min(model.cards.count - 1, Int((collectionView.contentOffset.y / h).rounded())))
        becomeCurrent(index, force: false)
    }

    private func becomeCurrent(_ index: Int, force: Bool) {
        guard let card = model.cards[safe: index], !sheetOpen else { return }
        if card.id == currentCardID && !force {
            playCurrent()
            return
        }
        if card.id != currentCardID {
            endView()
            current = index
            currentCardID = card.id
            becameCurrentAt = Date()
            ttffMs = nil
            watchedMs = 0
            loops = 0
            model.becameCurrent(index)
        }
        let mine = index % 2, other = 1 - mine
        players[other].pause()
        assigned[mine] = card.id
        assigned[other] = model.cards[safe: index + 1]?.id
        for case let cell as FeedCell in collectionView.visibleCells {
            if let ip = collectionView.indexPath(for: cell) { attach(to: cell, index: ip.item) }
        }
        bar.configure(card)
        playCurrent()
    }

    private func playCurrent() {
        guard let card = model.cards[safe: current], !sheetOpen,
              UIApplication.shared.applicationState != .background else { return }
        players[current % 2].play(card.youtubeID)
    }

    private func attach(to cell: FeedCell, index: Int) {
        guard let card = model.cards[safe: index] else { return }
        for (slot, id) in assigned.enumerated() where id == card.id {
            let player = players[slot]
            let changing = player.videoID != card.youtubeID
            cell.host(player.webView)
            if changing { cell.showPlayer(false) }
            if index != current { player.cue(card.youtubeID) }
        }
    }

    private func slot(of webView: WKWebView) -> Int? {
        players.firstIndex { $0.webView === webView }
    }

    private func pauseCurrent(endView end: Bool) {
        players[current % 2].pause()
        if end { endView() }
    }

    /// view_end for the clip that was current.
    private func endView() {
        guard let id = currentCardID, let card = model.cards.first(where: { $0.id == id }) else { return }
        if let since = playingSince { watchedMs += Int(Date().timeIntervalSince(since) * 1000) }
        playingSince = nil
        if watchedMs > 0 || ttffMs != nil {
            model.viewEnded(card, watchedMs: watchedMs, loops: loops, ttffMs: ttffMs)
        }
        watchedMs = 0
        loops = 0
        ttffMs = nil
        currentCardID = nil
    }

    // MARK: Player events

    private func handle(_ player: YouTubePlayer, videoID: String?, state: YTState) {
        // Swap the placeholder for the player once it actually plays.
        if state == .playing, let cell = cell(hosting: player.webView) { cell.showPlayer(true) }

        guard let card = model.cards[safe: current], player === players[current % 2], videoID == card.youtubeID else {
            // Never two audio tracks: anything else that starts playing is paused.
            if state == .playing { player.pause() }
            return
        }
        switch state {
        case .playing:
            if ttffMs == nil { ttffMs = Int(Date().timeIntervalSince(becameCurrentAt) * 1000) }
            if playingSince == nil { playingSince = Date() }
            if let next = model.cards[safe: current + 1] { players[(current + 1) % 2].cue(next.youtubeID) }
        case .paused, .buffering, .ended, .error:
            if let since = playingSince { watchedMs += Int(Date().timeIntervalSince(since) * 1000) }
            playingSince = nil
            if state == .ended {
                loops += 1
                player.seekToStartAndPlay()
            }
            if case .error(let code) = state { model.playerError(card, code: code) }
        default:
            break
        }
    }

    private func cell(hosting webView: WKWebView) -> FeedCell? {
        collectionView.visibleCells.compactMap { $0 as? FeedCell }.first { $0.hostedWebView === webView }
    }

    // MARK: Bar actions

    private func wireBar() {
        bar.onLike = { [weak self] in
            guard let self, let card = self.model.current else { return }
            self.model.toggleLike(card.id)
        }
        bar.onSave = { [weak self] in
            guard let self, let card = self.model.current else { return }
            self.model.toggleSave(card.id)
        }
        bar.onUndo = { [weak self] in self?.model.undo() }
        bar.onChannel = { [weak self] in
            guard let self, let card = self.model.current else { return }
            self.pauseCurrent(endView: false)
            self.openURL(card.channelURL)
        }
        bar.onShare = { [weak self] in
            guard let self, let card = self.model.current else { return }
            self.model.shared(card)
            let sheet = UIActivityViewController(activityItems: [card.shareURL], applicationActivities: nil)
            sheet.completionWithItemsHandler = { [weak self] _, _, _, _ in self?.sheetClosed() }
            self.presentSheet(sheet)
        }
        bar.onMore = { [weak self] in
            guard let self, let card = self.model.current else { return }
            let more = MoreSheet(card: card, languageName: self.languageName) { [weak self] action in
                guard let self else { return }
                self.dismiss(animated: true) { self.sheetClosed() }
                switch action {
                case .hide: self.model.hide(card)
                case .blockChannel: self.model.blockChannel(card)
                case .report(let reason): self.model.report(card, reason: reason)
                case .cancel: break
                }
            }
            let host = UIHostingController(rootView: more)
            host.view.backgroundColor = UIColor(Palette.surfaceRaised)
            if let sheet = host.sheetPresentationController {
                sheet.detents = [.medium(), .large()]
                sheet.prefersGrabberVisible = true
                sheet.preferredCornerRadius = 22
            }
            host.presentationController?.delegate = self
            self.presentSheet(host)
        }
    }

    var languageName: String {
        Locale.current.localizedString(forLanguageCode: model.language) ?? model.language
    }

    /// Any sheet pauses the clip first; it resumes when the sheet closes.
    private func presentSheet(_ vc: UIViewController) {
        pauseCurrent(endView: false)
        sheetOpen = true
        present(vc, animated: true)
    }

    fileprivate func sheetClosed() {
        sheetOpen = false
        becomeCurrent(current, force: true)
    }
}

extension FeedViewController: UIAdaptivePresentationControllerDelegate {
    func presentationControllerDidDismiss(_ presentationController: UIPresentationController) {
        sheetClosed()
    }
}

extension Array {
    subscript(safe i: Int) -> Element? { indices.contains(i) ? self[i] : nil }
}

/// SwiftUI wrapper. Rebuilt (new players) when the language or captions setting changes.
struct FeedContainer: UIViewControllerRepresentable {
    let model: FeedModel
    @Environment(\.openURL) private var openURL

    func makeUIViewController(context: Context) -> FeedViewController {
        let open = openURL
        return FeedViewController(model: model) { open($0) }
    }

    func updateUIViewController(_ vc: FeedViewController, context: Context) {}
}
