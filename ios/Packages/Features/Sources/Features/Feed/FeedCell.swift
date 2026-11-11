import Core
import UIKit
import WebKit

/// One page of the feed: only the player area. The player frame is the largest rect at the
/// clip's aspect that fits, centred on black. Inside it there is either our placeholder or the
/// pooled web view, never both visible and never anything of ours on top of the player.
final class FeedCell: UICollectionViewCell {
    static let minPlayerSide: CGFloat = 200

    let playerFrame = UIView()
    private let placeholder = UIImageView()
    private var aspect: CGFloat = 9.0 / 16.0
    private var youtubeID = ""
    private var thumbTask: URLSessionDataTask?
    private(set) weak var hostedWebView: WKWebView?

    override init(frame: CGRect) {
        super.init(frame: frame)
        contentView.backgroundColor = .black
        playerFrame.backgroundColor = .black
        playerFrame.clipsToBounds = true
        playerFrame.accessibilityIdentifier = "playerFrame"
        contentView.addSubview(playerFrame)

        placeholder.backgroundColor = UIColor(red: 0x11 / 255, green: 0x11 / 255, blue: 0x13 / 255, alpha: 1)
        placeholder.contentMode = .scaleAspectFill
        placeholder.clipsToBounds = true
        placeholder.isAccessibilityElement = false
        playerFrame.addSubview(placeholder)
        disableCellMenuGesture()
    }

    /// UIKit gives every cell a long-press for context menus. The cell is the player's
    /// frame, so no touch action of ours may live there: switch it off.
    private func disableCellMenuGesture() {
        for g in contentView.gestureRecognizers ?? [] where g is UILongPressGestureRecognizer || g is UITapGestureRecognizer {
            g.isEnabled = false
        }
    }

    required init?(coder: NSCoder) { fatalError() }

    override func layoutSubviews() {
        super.layoutSubviews()
        playerFrame.frame = Self.fit(aspect: aspect, in: contentView.bounds)
        placeholder.frame = playerFrame.bounds
        hostedWebView?.frame = playerFrame.bounds
    }

    /// Largest rect at `aspect` (width / height) inside `bounds`, centred, pixel-aligned.
    static func fit(aspect: CGFloat, in bounds: CGRect) -> CGRect {
        guard aspect > 0, bounds.width > 0, bounds.height > 0 else { return .zero }
        var w = bounds.width
        var h = w / aspect
        if h > bounds.height { h = bounds.height; w = h * aspect }
        return CGRect(x: bounds.minX + (bounds.width - w) / 2, y: bounds.minY + (bounds.height - h) / 2, width: w, height: h).integral
    }

    func configure(_ card: VideoCard) {
        disableCellMenuGesture()
        aspect = CGFloat(card.aspect > 0 ? card.aspect : 9.0 / 16.0)
        accessibilityLabel = card.title
        guard card.youtubeID != youtubeID else { setNeedsLayout(); return }
        youtubeID = card.youtubeID
        placeholder.image = nil
        thumbTask?.cancel()
        let id = card.youtubeID
        thumbTask = URLSession.shared.dataTask(with: card.thumbnailURL) { [weak self] data, _, _ in
            guard let data, let image = UIImage(data: data) else { return }
            DispatchQueue.main.async {
                guard let self, self.youtubeID == id else { return }
                self.placeholder.image = image
            }
        }
        thumbTask?.resume()
        showPlayer(false)
        setNeedsLayout()
    }

    func host(_ webView: WKWebView) {
        if webView.superview !== playerFrame {
            webView.removeFromSuperview()
            playerFrame.addSubview(webView)
        }
        hostedWebView = webView
        webView.frame = playerFrame.bounds
    }

    func releaseWebView() {
        hostedWebView?.removeFromSuperview()
        hostedWebView = nil
        showPlayer(false)
    }

    /// Swap, never stack: the web view stays in the hierarchy (so it keeps loading and
    /// can start) but is transparent until it plays; then the placeholder goes away.
    func showPlayer(_ show: Bool) {
        hostedWebView?.alpha = show ? 1 : 0
        placeholder.isHidden = show
    }
}
