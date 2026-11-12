import XCTest
@testable import Core

final class InstallIDTests: XCTestCase {
    func testCreatedOnceAndStable() {
        let kc = InMemoryKeychain()
        let first = InstallID.current(in: kc)
        XCTAssertEqual(InstallID.current(in: kc), first)
        XCTAssertGreaterThanOrEqual(first.count, 16, "server requires 16 to 64 chars")
        XCTAssertNotNil(UUID(uuidString: first))
    }

    func testDifferentKeychainsGetDifferentIDs() {
        XCTAssertNotEqual(InstallID.current(in: InMemoryKeychain()), InstallID.current(in: InMemoryKeychain()))
    }
}

// MARK: - Session

private final class FakeSessionAPI: SessionAPI, @unchecked Sendable {
    var requests: [SessionRequest] = []
    var error: APIError?
    var tokenCounter = 0
    let lock = NSLock()

    func createSession(_ request: SessionRequest) async throws -> SessionResult {
        try lock.withLock {
            requests.append(request)
            if let error { throw error }
            tokenCounter += 1
            let user = UserProfile(id: "user-1", ageBand: "adult", uiLanguage: request.uiLanguage,
                                   nativeLanguage: request.nativeLanguage, learning: request.learning)
            return SessionResult(token: "token-\(tokenCounter)", user: user)
        }
    }
}

private final class Counter: @unchecked Sendable {
    var tokens: [String] = []
    let lock = NSLock()
    func record(_ t: String) -> Int { lock.withLock { tokens.append(t); return tokens.count } }
}

final class SessionManagerTests: XCTestCase {
    private let answers = OnboardingAnswers(birthYear: 1990, learning: "es", native: "en", level: 3)

    func testRegisterStoresTokenAndSendsAnswers() async throws {
        let kc = InMemoryKeychain(), api = FakeSessionAPI()
        let manager = SessionManager(api: api, keychain: kc, uiLanguage: "en")
        let user = try await manager.register(answers)

        XCTAssertEqual(user.activeLanguage, "es")
        let token = await manager.token
        XCTAssertEqual(token, "token-1")
        let registered = await manager.isRegistered
        XCTAssertTrue(registered)
        XCTAssertEqual(api.requests.first?.installID, InstallID.current(in: kc))
        XCTAssertEqual(api.requests.first?.learning, [LearningLanguage(language: "es", level: 3, isActive: true)])
    }

    func testUnauthorizedReRegistersWithSameInstallIDAndRetriesOnce() async throws {
        let kc = InMemoryKeychain(), api = FakeSessionAPI()
        let manager = SessionManager(api: api, keychain: kc, uiLanguage: "en")
        _ = try await manager.register(answers)

        let counter = Counter()
        let result = try await manager.authorized { token -> String in
            if counter.record(token) == 1 { throw APIError.unauthorized }
            return "ok with \(token)"
        }

        XCTAssertEqual(result, "ok with token-2")
        XCTAssertEqual(counter.tokens, ["token-1", "token-2"])
        XCTAssertEqual(api.requests.count, 2)
        XCTAssertEqual(api.requests[0].installID, api.requests[1].installID)
    }

    func testSecondUnauthorizedIsNotRetriedAgain() async throws {
        let manager = SessionManager(api: FakeSessionAPI(), keychain: InMemoryKeychain(), uiLanguage: "en")
        _ = try await manager.register(answers)
        let counter = Counter()
        do {
            _ = try await manager.authorized { token -> Int in
                _ = counter.record(token)
                throw APIError.unauthorized
            }
            XCTFail("expected unauthorized")
        } catch {
            XCTAssertEqual(error as? APIError, .unauthorized)
        }
        XCTAssertEqual(counter.tokens.count, 2)
    }

    func testAgeBlockedIsStoredAndSurvivesReset() async throws {
        let kc = InMemoryKeychain(), api = FakeSessionAPI()
        api.error = .ageBlocked
        let manager = SessionManager(api: api, keychain: kc, uiLanguage: "en")
        do {
            _ = try await manager.register(OnboardingAnswers(birthYear: 2016, learning: "es", native: "en", level: 1))
            XCTFail("expected ageBlocked")
        } catch {
            XCTAssertEqual(error as? APIError, .ageBlocked)
        }
        XCTAssertTrue(AgeBlock.isBlocked(in: kc))
        await manager.reset()
        XCTAssertTrue(AgeBlock.isBlocked(in: kc))
    }

    func testResetForgetsTokenAndInstallID() async throws {
        let kc = InMemoryKeychain()
        let manager = SessionManager(api: FakeSessionAPI(), keychain: kc, uiLanguage: "en")
        _ = try await manager.register(answers)
        let before = InstallID.current(in: kc)
        await manager.reset()
        let registered = await manager.isRegistered
        XCTAssertFalse(registered)
        XCTAssertNotEqual(InstallID.current(in: kc), before)
    }
}

// MARK: - Events

private final class FakeSink: EventSink, @unchecked Sendable {
    var failuresLeft = 0
    var sent: [[ClientEvent]] = []
    let lock = NSLock()

    func send(_ events: [ClientEvent]) async throws {
        try lock.withLock {
            sent.append(events)
            if failuresLeft > 0 {
                failuresLeft -= 1
                throw APIError.offline
            }
        }
    }
}

final class EventLoggerTests: XCTestCase {
    private func tempFile() -> URL {
        FileManager.default.temporaryDirectory.appendingPathComponent("events-\(UUID().uuidString).json")
    }

    func testFailedFlushRetriesWithSameIDs() async {
        let sink = FakeSink()
        sink.failuresLeft = 1
        let logger = BatchingEventLogger(sink: sink, fileURL: nil)
        await logger.append(ClientEvent(type: .impression, videoID: "v1", props: ["position": .int(0)]))
        await logger.append(ClientEvent(type: .viewEnd, videoID: "v1", props: ["completed": .bool(true), "watched_ms": .int(1500)]))

        await logger.flush()
        let afterFailure = await logger.pendingEvents
        XCTAssertEqual(afterFailure.count, 2)

        await logger.flush()
        let afterSuccess = await logger.pendingEvents
        XCTAssertTrue(afterSuccess.isEmpty)
        XCTAssertEqual(sink.sent.count, 2)
        XCTAssertEqual(sink.sent[0].map(\.id), sink.sent[1].map(\.id))
    }

    func testPendingEventsSurviveRelaunchAndAreSentOnce() async {
        let file = tempFile()
        defer { try? FileManager.default.removeItem(at: file) }
        let event = ClientEvent(type: .share, videoID: "v2", props: [:])

        let first = BatchingEventLogger(sink: FakeSink(), fileURL: file)
        await first.append(event) // app killed before any flush

        let sink = FakeSink()
        let relaunched = BatchingEventLogger(sink: sink, fileURL: file)
        await relaunched.flush()
        await relaunched.flush()
        XCTAssertEqual(sink.sent.flatMap { $0 }.map(\.id), [event.id])
        XCTAssertFalse(FileManager.default.fileExists(atPath: file.path))
    }

    func testBatchesAreCappedAt200() async {
        let sink = FakeSink()
        let logger = BatchingEventLogger(sink: sink, fileURL: nil)
        for i in 0..<450 { await logger.append(ClientEvent(type: .impression, videoID: nil, props: ["position": .int(i)])) }
        await logger.flush()
        XCTAssertEqual(sink.sent.map(\.count), [200, 200, 50])
    }

    // Reset my data: queued events of the erased install are dropped, file included.
    func testDiscardAllDropsPendingEventsAndFile() async {
        let file = tempFile()
        defer { try? FileManager.default.removeItem(at: file) }
        let sink = FakeSink()
        let logger = BatchingEventLogger(sink: sink, fileURL: file)
        await logger.append(ClientEvent(type: .share, videoID: "v3", props: [:]))
        await logger.discardAll()
        await logger.flush()
        XCTAssertTrue(sink.sent.isEmpty)
        XCTAssertFalse(FileManager.default.fileExists(atPath: file.path))
    }

    func testCompletedEncodesAsJSONBoolean() throws {
        let data = try JSONEncoder().encode(ClientEvent(type: .viewEnd, videoID: nil, props: ["completed": .bool(false)]).props)
        XCTAssertEqual(String(data: data, encoding: .utf8), #"{"completed":false}"#)
    }
}

final class LevelTests: XCTestCase {
    func testCEFRAndWords() {
        XCTAssertEqual(Level.cefr(1), "A1")
        XCTAssertEqual(Level.cefr(6), "C2")
        XCTAssertEqual(Level.plain(3), "Can get by")
    }
}
