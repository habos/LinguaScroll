import Foundation

public enum EventType: String, Codable, Sendable {
    case impression
    case viewEnd = "view_end"
    case share
    case playerError = "player_error"
}

/// A JSON scalar for event props. `completed` must be a real boolean (feed ranking reads it).
public enum JSONValue: Codable, Sendable, Equatable {
    case int(Int)
    case double(Double)
    case bool(Bool)
    case string(String)

    public init(from decoder: any Decoder) throws {
        let c = try decoder.singleValueContainer()
        if let b = try? c.decode(Bool.self) { self = .bool(b) }
        else if let i = try? c.decode(Int.self) { self = .int(i) }
        else if let d = try? c.decode(Double.self) { self = .double(d) }
        else { self = .string(try c.decode(String.self)) }
    }

    public func encode(to encoder: any Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .int(let v): try c.encode(v)
        case .double(let v): try c.encode(v)
        case .bool(let v): try c.encode(v)
        case .string(let v): try c.encode(v)
        }
    }
}

public struct ClientEvent: Codable, Sendable, Equatable, Identifiable {
    /// Client-generated; retries reuse it so the server can deduplicate.
    public var id: UUID
    public var type: EventType
    public var at: Date
    public var videoID: String?
    public var props: [String: JSONValue]

    public init(id: UUID = UUID(), type: EventType, at: Date = Date(), videoID: String?, props: [String: JSONValue]) {
        self.id = id
        self.type = type
        self.at = at
        self.videoID = videoID
        self.props = props
    }
}

/// Where batches go (`POST /v1/events`). Throws to keep the batch for a retry.
public protocol EventSink: Sendable {
    func send(_ events: [ClientEvent]) async throws
}

/// Spec interface (V5).
public protocol EventLogger: Sendable {
    func log(_ type: EventType, videoID: String?, props: [String: JSONValue])
}

/// Batches events in memory and in a file, flushes every `interval` seconds and
/// whenever `flush()` is called (the app calls it on backgrounding). A failed
/// send keeps the events, with the same IDs, for the next flush; a crash or kill
/// loses nothing that reached the file.
public actor BatchingEventLogger: EventLogger {
    public static let maxBatch = 200

    private let sink: any EventSink
    private let fileURL: URL?
    private var pending: [ClientEvent] = []
    private var timer: Task<Void, Never>?
    private var flushing = false

    public init(sink: any EventSink, fileURL: URL?) {
        self.sink = sink
        self.fileURL = fileURL
        if let fileURL, let data = try? Data(contentsOf: fileURL),
           let saved = try? Self.decoder.decode([ClientEvent].self, from: data) {
            pending = saved
        }
    }

    /// Default file in Application Support.
    public static func defaultFileURL() -> URL? {
        guard let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first else { return nil }
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("pending-events.json")
    }

    public nonisolated func log(_ type: EventType, videoID: String?, props: [String: JSONValue]) {
        let event = ClientEvent(type: type, videoID: videoID, props: props)
        Task { await self.append(event) }
    }

    public func append(_ event: ClientEvent) {
        pending.append(event)
        persist()
    }

    public var pendingEvents: [ClientEvent] { pending }

    /// Reset my data: events from the erased install must not be sent under the new one.
    public func discardAll() {
        pending = []
        persist()
    }

    public func start(interval: Duration = .seconds(10)) {
        timer?.cancel()
        timer = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: interval)
                await self?.flush()
            }
        }
    }

    public func stop() {
        timer?.cancel()
        timer = nil
    }

    /// Sends everything pending, in batches of at most 200. Stops at the first failure.
    public func flush() async {
        guard !flushing else { return }
        flushing = true
        defer { flushing = false }
        while !pending.isEmpty {
            let batch = Array(pending.prefix(Self.maxBatch))
            do {
                try await sink.send(batch)
            } catch {
                return
            }
            let sent = Set(batch.map(\.id))
            pending.removeAll { sent.contains($0.id) }
            persist()
        }
    }

    private func persist() {
        guard let fileURL else { return }
        if pending.isEmpty {
            try? FileManager.default.removeItem(at: fileURL)
        } else if let data = try? Self.encoder.encode(pending) {
            try? data.write(to: fileURL, options: .atomic)
        }
    }

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .iso8601
        return e
    }()

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        return d
    }()
}
