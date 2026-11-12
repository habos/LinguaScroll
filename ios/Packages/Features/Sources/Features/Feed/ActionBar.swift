import Core
import UIKit

/// The bar pinned under the player area: channel column (or a 2.5 s message) and
/// like, save, share, more. It never overlaps the player.
final class ActionBar: UIView {
    static let minHeight: CGFloat = 64

    var onChannel: (() -> Void)?
    var onLike: (() -> Void)?
    var onSave: (() -> Void)?
    var onShare: (() -> Void)?
    var onMore: (() -> Void)?
    var onUndo: (() -> Void)?

    private let channelButton = UIButton(type: .system)
    private let channelTitle = UILabel()
    private let channelSub = UILabel()
    private let messageLine1 = UILabel()
    private let messageLine2 = UIButton(type: .system)
    private let channelColumn = UIStackView()
    private let messageColumn = UIStackView()
    private var like: UIButton!
    private var save: UIButton!
    private var share: UIButton!
    private var more: UIButton!
    private var liked = false

    private static let accent = UIColor(red: 0x22 / 255, green: 0xD3 / 255, blue: 0xC5 / 255, alpha: 1)
    private static let secondary = UIColor(red: 0x9F / 255, green: 0xB3 / 255, blue: 0xB5 / 255, alpha: 1)
    private static let destructive = UIColor(red: 1, green: 0x7A / 255, blue: 0x66 / 255, alpha: 1)

    override init(frame: CGRect) {
        super.init(frame: frame)
        backgroundColor = .black

        channelTitle.font = UIFont.preferredFont(forTextStyle: .subheadline).withWeight(.semibold)
        channelTitle.adjustsFontForContentSizeCategory = true
        channelTitle.textColor = .white
        channelSub.font = .preferredFont(forTextStyle: .caption1)
        channelSub.adjustsFontForContentSizeCategory = true
        channelSub.textColor = Self.secondary
        channelSub.text = String(localized: "Open on YouTube ↗")
        channelColumn.axis = .vertical
        channelColumn.spacing = 2
        channelColumn.isUserInteractionEnabled = false
        channelColumn.addArrangedSubview(channelTitle)
        channelColumn.addArrangedSubview(channelSub)
        channelButton.addSubview(channelColumn)
        channelColumn.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            channelColumn.leadingAnchor.constraint(equalTo: channelButton.leadingAnchor),
            channelColumn.trailingAnchor.constraint(equalTo: channelButton.trailingAnchor),
            channelColumn.centerYAnchor.constraint(equalTo: channelButton.centerYAnchor),
            channelButton.heightAnchor.constraint(greaterThanOrEqualTo: channelColumn.heightAnchor),
            channelButton.heightAnchor.constraint(greaterThanOrEqualToConstant: 44),
        ])
        channelButton.addAction(UIAction { [weak self] _ in self?.onChannel?() }, for: .touchUpInside)

        messageLine1.font = UIFont.preferredFont(forTextStyle: .subheadline).withWeight(.semibold)
        messageLine1.adjustsFontForContentSizeCategory = true
        messageLine2.titleLabel?.font = .preferredFont(forTextStyle: .caption1)
        messageLine2.contentHorizontalAlignment = .leading
        messageLine2.addAction(UIAction { [weak self] _ in self?.onUndo?() }, for: .touchUpInside)
        messageColumn.axis = .vertical
        messageColumn.alignment = .leading
        messageColumn.spacing = 2
        messageColumn.addArrangedSubview(messageLine1)
        messageColumn.addArrangedSubview(messageLine2)
        messageColumn.isHidden = true

        like = icon("heart", label: String(localized: "Like")) { [weak self] in self?.onLike?() }
        save = icon("bookmark", label: String(localized: "Save")) { [weak self] in self?.onSave?() }
        share = icon("square.and.arrow.up", label: String(localized: "Share")) { [weak self] in self?.onShare?() }
        more = icon("ellipsis", label: String(localized: "More options")) { [weak self] in self?.onMore?() }

        let left = UIStackView(arrangedSubviews: [channelButton, messageColumn])
        left.axis = .vertical
        left.setContentHuggingPriority(.defaultLow, for: .horizontal)
        let icons = UIStackView(arrangedSubviews: [like, save, share, more])
        icons.spacing = 4
        icons.setContentHuggingPriority(.required, for: .horizontal)
        icons.setContentCompressionResistancePriority(.required, for: .horizontal)
        let row = UIStackView(arrangedSubviews: [left, icons])
        row.alignment = .center
        row.spacing = 8
        row.translatesAutoresizingMaskIntoConstraints = false
        addSubview(row)
        NSLayoutConstraint.activate([
            row.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 16),
            row.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8),
            row.topAnchor.constraint(greaterThanOrEqualTo: topAnchor, constant: 4),
            row.bottomAnchor.constraint(lessThanOrEqualTo: bottomAnchor, constant: -4),
            row.centerYAnchor.constraint(equalTo: centerYAnchor),
            heightAnchor.constraint(greaterThanOrEqualToConstant: Self.minHeight),
        ])
    }

    required init?(coder: NSCoder) { fatalError() }

    func configure(_ card: VideoCard?) {
        isHidden = card == nil
        guard let card else { return }
        channelTitle.text = card.channelTitle
        channelButton.accessibilityLabel = String(localized: "\(card.channelTitle), opens in YouTube")
        setIcon(like, card.liked ? "heart.fill" : "heart", on: card.liked)
        like.accessibilityLabel = card.liked ? String(localized: "Liked") : String(localized: "Like")
        like.accessibilityTraits = card.liked ? [.button, .selected] : .button
        setIcon(save, card.saved ? "bookmark.fill" : "bookmark", on: card.saved)
        save.accessibilityLabel = card.saved ? String(localized: "Saved") : String(localized: "Save")
        save.accessibilityTraits = card.saved ? [.button, .selected] : .button
        if card.liked && !liked { UIImpactFeedbackGenerator(style: .light).impactOccurred() }
        liked = card.liked
    }

    func show(_ message: BarMessage?) {
        channelButton.isHidden = message != nil
        messageColumn.isHidden = message == nil
        guard let message else { return }
        messageLine1.text = message.line1
        messageLine1.textColor = message.kind == .actionFailed ? Self.destructive : .white
        messageLine2.setTitle(message.line2, for: .normal)
        let isUndo = message.kind == .hidden
        messageLine2.isUserInteractionEnabled = isUndo
        messageLine2.setTitleColor(isUndo ? Self.accent : Self.secondary, for: .normal)
        UIAccessibility.post(notification: .announcement, argument: "\(message.line1). \(message.line2)")
    }

    private func icon(_ symbol: String, label: String, action: @escaping () -> Void) -> UIButton {
        var config = UIButton.Configuration.plain()
        config.image = UIImage(systemName: symbol)
        config.preferredSymbolConfigurationForImage = .init(textStyle: .title2)
        config.baseForegroundColor = .white
        let button = UIButton(configuration: config, primaryAction: UIAction { _ in action() })
        button.accessibilityLabel = label
        NSLayoutConstraint.activate([
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: 44),
            button.heightAnchor.constraint(greaterThanOrEqualToConstant: 44),
        ])
        return button
    }

    private func setIcon(_ button: UIButton, _ symbol: String, on: Bool) {
        button.configuration?.image = UIImage(systemName: symbol)
        button.configuration?.baseForegroundColor = on ? Self.accent : .white
    }
}

private extension UIFont {
    func withWeight(_ weight: UIFont.Weight) -> UIFont {
        let d = fontDescriptor.addingAttributes([.traits: [UIFontDescriptor.TraitKey.weight: weight]])
        return UIFont(descriptor: d, size: 0)
    }
}
