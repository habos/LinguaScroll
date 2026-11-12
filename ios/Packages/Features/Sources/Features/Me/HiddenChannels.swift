import Foundation

/// Channels this device hid ("Don't show this channel"), newest first, kept on the phone.
/// The server only stores channel IDs; `GET /v1/channel-blocks` (backend V4) will replace this list.
public struct HiddenChannels {
    public struct Entry: Codable, Equatable, Identifiable, Sendable {
        public var id: String          // YouTube channel ID
        public var title: String?
        public var hiddenAt: Date
    }

    let defaults: UserDefaults
    static let key = "hidden_channels"

    public var all: [Entry] {
        guard let data = defaults.data(forKey: Self.key),
              let list = try? JSONDecoder().decode([Entry].self, from: data) else { return [] }
        return list.sorted { $0.hiddenAt > $1.hiddenAt }
    }

    public func add(_ id: String, title: String?) {
        save([Entry(id: id, title: title, hiddenAt: Date())] + all.filter { $0.id != id })
    }

    public func remove(_ id: String) {
        save(all.filter { $0.id != id })
    }

    public func clear() {
        defaults.removeObject(forKey: Self.key)
    }

    private func save(_ list: [Entry]) {
        if let data = try? JSONEncoder().encode(list) { defaults.set(data, forKey: Self.key) }
    }
}
