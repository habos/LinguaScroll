import Core
import Features
import SwiftUI

@main
struct LinguaScrollApp: App {
    @Environment(\.scenePhase) private var scenePhase
    private let model: AppModel

    init() {
        PlaybackAudio.configure()
        let baseURL = (Bundle.main.object(forInfoDictionaryKey: "APIBaseURL") as? String)
            .flatMap(URL.init(string:)) ?? URL(string: "http://localhost:8080")!
        let keychain = SystemKeychain.shared
        let api = LiveAPI(baseURL: baseURL)
        let uiLanguage = Locale.current.language.languageCode?.identifier ?? "en"
        let session = SessionManager(api: api, keychain: keychain, uiLanguage: uiLanguage)
        let events = BatchingEventLogger(
            sink: LiveEventSink(api: api, session: session),
            fileURL: BatchingEventLogger.defaultFileURL()
        )
        model = AppModel(deps: AppDependencies(
            keychain: keychain, session: session, catalog: api, users: api, feed: api, events: events
        ))
        Task { await events.start() }
    }

    var body: some Scene {
        WindowGroup { RootView(model: model) }
            .onChange(of: scenePhase) { _, phase in
                if phase == .background {
                    let events = model.deps.events
                    Task { await events.flush() }
                }
            }
    }
}
