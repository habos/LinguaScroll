import UIKit
import WebKit

/// One clip: the player frame (fit at the clip's aspect, centred, black around it)
/// and the action bar pinned to the bottom. Nothing is ever added inside the
/// player frame except the pooled web view and, behind it, the thumbnail.
final class FeedCell: UICollectionViewCell {
    static let barHeight: CGFloat = 64
    static let aspect: CGFloat = 9.0 / 16.0 // width / height; the real app uses the card's aspect

    let playerFrame = UIView()
    private let thumbnail = UIImageView()
    private let bar = UIStackView()
    private let channel = UILabel()
    private var likeButton: UIButton!
    private var saveButton: UIButton!
    private var liked = false
    private var saved = false
    private var videoID = ""
    private var thumbTask: URLSessionDataTask?

    private(set) weak var hostedWebView: WKWebView?

    override init(frame: CGRect) {
        super.init(frame: frame)
        contentView.backgroundColor = .black

        playerFrame.backgroundColor = .black
        playerFrame.clipsToBounds = true
        contentView.addSubview(playerFrame)

        thumbnail.contentMode = .scaleAspectFill
        thumbnail.clipsToBounds = true
        playerFrame.addSubview(thumbnail)

        channel.textColor = .white
        channel.font = .preferredFont(forTextStyle: .subheadline)
        channel.adjustsFontForContentSizeCategory = true
        channel.lineBreakMode = .byTruncatingTail
        channel.setContentHuggingPriority(.defaultLow, for: .horizontal)

        likeButton = makeButton("heart") { [weak self] in self?.toggleLike() }
        saveButton = makeButton("bookmark") { [weak self] in self?.toggleSave() }
        let share = makeButton("square.and.arrow.up") { [weak self] in self?.share() }
        let more = makeButton("ellipsis") {}

        bar.axis = .horizontal
        bar.alignment = .center
        bar.spacing = 4
        bar.isLayoutMarginsRelativeArrangement = true
        bar.directionalLayoutMargins = .init(top: 0, leading: 16, bottom: 0, trailing: 8)
        for v in [channel, likeButton!, saveButton!, share, more] as [UIView] { bar.addArrangedSubview(v) }
        contentView.addSubview(bar)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func layoutSubviews() {
        super.layoutSubviews()
        let b = contentView.bounds
        bar.frame = CGRect(x: 0, y: b.height - Self.barHeight, width: b.width, height: Self.barHeight)

        // Largest rect at the clip's aspect that fits above the bar, centred.
        let area = CGRect(x: 0, y: 0, width: b.width, height: b.height - Self.barHeight)
        var w = area.width
        var h = w / Self.aspect
        if h > area.height { h = area.height; w = h * Self.aspect }
        playerFrame.frame = CGRect(x: (area.width - w) / 2, y: (area.height - h) / 2, width: w, height: h).integral
        thumbnail.frame = playerFrame.bounds
        hostedWebView?.frame = playerFrame.bounds
    }

    func configure(videoID: String, index: Int) {
        self.videoID = videoID
        channel.text = "#\(index + 1) · \(videoID)"
        liked = false
        saved = false
        updateButtons()
        thumbnail.image = nil
        thumbTask?.cancel()
        if let url = URL(string: "https://i.ytimg.com/vi/\(videoID)/hqdefault.jpg") {
            thumbTask = URLSession.shared.dataTask(with: url) { [weak self] data, _, _ in
                guard let data, let img = UIImage(data: data) else { return }
                DispatchQueue.main.async {
                    guard let self, self.videoID == videoID else { return }
                    self.thumbnail.image = img
                }
            }
            thumbTask?.resume()
        }
    }

    func host(_ webView: WKWebView) {
        if webView.superview !== playerFrame {
            webView.removeFromSuperview()
            playerFrame.addSubview(webView) // above the thumbnail
        }
        hostedWebView = webView
        webView.frame = playerFrame.bounds
    }

    // MARK: Bar

    private func makeButton(_ symbol: String, action: @escaping () -> Void) -> UIButton {
        var config = UIButton.Configuration.plain()
        config.image = UIImage(systemName: symbol)
        config.preferredSymbolConfigurationForImage = .init(pointSize: 22, weight: .regular)
        config.baseForegroundColor = .white
        let button = UIButton(configuration: config, primaryAction: UIAction { _ in action() })
        button.widthAnchor.constraint(equalToConstant: 44).isActive = true
        button.heightAnchor.constraint(equalToConstant: 44).isActive = true
        return button
    }

    private func toggleLike() { liked.toggle(); updateButtons() }
    private func toggleSave() { saved.toggle(); updateButtons() }

    private func updateButtons() {
        likeButton.configuration?.image = UIImage(systemName: liked ? "heart.fill" : "heart")
        likeButton.configuration?.baseForegroundColor = liked ? .systemPink : .white
        saveButton.configuration?.image = UIImage(systemName: saved ? "bookmark.fill" : "bookmark")
    }

    private func share() {
        guard let url = URL(string: "https://youtube.com/shorts/\(videoID)"),
              let vc = window?.rootViewController else { return }
        var top = vc
        while let presented = top.presentedViewController { top = presented }
        top.present(UIActivityViewController(activityItems: [url], applicationActivities: nil), animated: true)
    }
}
