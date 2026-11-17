import Foundation
import Security

/// Small key-value view of the Keychain so tests can use a fake.
public protocol KeychainStore: Sendable {
    func data(for key: String) -> Data?
    func set(_ data: Data?, for key: String)
}

public extension KeychainStore {
    func string(for key: String) -> String? {
        data(for: key).flatMap { String(data: $0, encoding: .utf8) }
    }

    func setString(_ value: String?, for key: String) {
        set(value.map { Data($0.utf8) }, for: key)
    }
}

/// Generic-password items, this device only, never synced to iCloud.
/// Items survive deleting and reinstalling the app, but not a new phone.
public struct SystemKeychain: KeychainStore {
    public static let shared = SystemKeychain(service: "app.linguascroll")
    let service: String

    public init(service: String) { self.service = service }

    private func query(_ key: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecAttrSynchronizable as String: false,
        ]
    }

    public func data(for key: String) -> Data? {
        var q = query(key)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess else { return nil }
        return out as? Data
    }

    public func set(_ data: Data?, for key: String) {
        let q = query(key)
        guard let data else {
            SecItemDelete(q as CFDictionary)
            return
        }
        let attrs: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        if SecItemUpdate(q as CFDictionary, attrs as CFDictionary) == errSecItemNotFound {
            SecItemAdd(q.merging(attrs) { $1 } as CFDictionary, nil)
        }
    }
}

/// In-memory Keychain for tests and previews.
public final class InMemoryKeychain: KeychainStore, @unchecked Sendable {
    private var items: [String: Data] = [:]
    private let lock = NSLock()

    public init() {}

    public func data(for key: String) -> Data? {
        lock.withLock { items[key] }
    }

    public func set(_ data: Data?, for key: String) {
        lock.withLock { items[key] = data }
    }
}

/// Keychain keys used by the app.
public enum KeychainKey {
    public static let installID = "install_id"
    public static let sessionToken = "session_token"
    public static let sessionRequest = "session_request"
    public static let ageBlocked = "age_blocked"
}

/// The anonymous identity of this install: a random UUID created on first launch.
public enum InstallID {
    public static func current(in keychain: any KeychainStore = SystemKeychain.shared) -> String {
        if let id = keychain.string(for: KeychainKey.installID) { return id }
        let id = UUID().uuidString.lowercased()
        keychain.setString(id, for: KeychainKey.installID)
        return id
    }
}

/// Under-13 block. Stored in the Keychain so it survives reinstalls; never cleared by reset.
public enum AgeBlock {
    public static func isBlocked(in keychain: any KeychainStore = SystemKeychain.shared) -> Bool {
        keychain.string(for: KeychainKey.ageBlocked) == "1"
    }

    public static func set(in keychain: any KeychainStore = SystemKeychain.shared) {
        keychain.setString("1", for: KeychainKey.ageBlocked)
    }
}
