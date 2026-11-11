import Core
import SwiftUI

/// Feed tab: language pill on top, then the paging player area with its pinned bar,
/// or the loading / offline / caught-up state when there are no clips to show.
struct FeedScreen: View {
    @Environment(AppModel.self) private var app
    @Environment(\.scenePhase) private var scenePhase
    @State private var feed: FeedModel?
    @State private var showSwitcher = false

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Button { showSwitcher = true } label: { LanguagePill(code: app.activeLanguage) }
                    .buttonStyle(.plain)
                    .accessibilityHint(Text("Choose which language's clips to watch"))
                Spacer()
            }
            .frame(height: 44)
            .padding(.horizontal, 16)

            content
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .background(Palette.cinema.ignoresSafeArea(edges: .top))
        .foregroundStyle(Palette.textPrimary)
        .task(id: app.activeLanguage) {
            guard !app.activeLanguage.isEmpty else { return }
            let model = FeedModel(language: app.activeLanguage, deps: app.deps, captionsOn: app.config.youtubeCaptionsDefault)
            feed = model
            await model.start()
        }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active, let feed, feed.state != .ready { Task { await feed.retry() } }
        }
        .onChange(of: showSwitcher) { _, open in feed?.externallyPaused = open }
        .sheet(isPresented: $showSwitcher) {
            LanguageSwitcher().presentationDetents([.medium, .large]).presentationCornerRadius(22)
        }
    }

    @ViewBuilder private var content: some View {
        if let feed, !feed.cards.isEmpty {
            FeedContainer(model: feed).id(ObjectIdentifier(feed))
        } else if let feed, feed.state == .offline {
            StateView(title: String(localized: "You're offline"),
                      detail: String(localized: "We'll carry on as soon as you're back."),
                      button: String(localized: "Try again")) { Task { await feed.retry() } }
        } else if let feed, feed.state == .caughtUp {
            StateView(title: String(localized: "You've watched everything in \(languageName) for now"),
                      detail: String(localized: "New clips arrive every day. Meanwhile, try another language or rewatch what you saved."),
                      button: String(localized: "Switch language")) { showSwitcher = true }
        } else {
            VStack(spacing: 12) {
                ProgressView().tint(Palette.textPrimary)
                Text("Finding \(languageName) clips…").foregroundStyle(Palette.textSecondary)
            }
        }
    }

    private var languageName: String {
        app.language(app.activeLanguage)?.localizedName() ?? app.activeLanguage
    }
}

private struct StateView: View {
    let title: String
    let detail: String
    let button: String
    let action: () -> Void

    var body: some View {
        VStack(spacing: 12) {
            Text(verbatim: title).font(.title3.weight(.semibold)).multilineTextAlignment(.center)
            Text(verbatim: detail).foregroundStyle(Palette.textSecondary).multilineTextAlignment(.center)
            Button(action: action) { Text(verbatim: button) }
                .buttonStyle(PrimaryButtonStyle())
                .frame(maxWidth: 260)
                .padding(.top, 8)
        }
        .padding(24)
    }
}

/// "Watch clips in": the device's languages, tap to switch.
struct LanguageSwitcher: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Watch clips in").font(.title3.weight(.semibold)).padding(.bottom, 8)
            ForEach(app.user?.learning ?? [], id: \.language) { l in
                Button {
                    app.switchLanguage(to: l.language)
                    dismiss()
                } label: {
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(app.language(l.language)?.localizedName() ?? l.language).font(.body.weight(.semibold))
                            Text(verbatim: "\(Level.cefr(l.level)) · \(Level.plain(l.level))").font(.footnote).foregroundStyle(Palette.textSecondary)
                        }
                        Spacer()
                        if l.language == app.activeLanguage { Image(systemName: "checkmark").foregroundStyle(Palette.accent) }
                    }
                    .padding(.horizontal, 16)
                    .frame(minHeight: 60)
                    .background(l.language == app.activeLanguage ? Palette.accentTint : Palette.surface, in: RoundedRectangle(cornerRadius: 14))
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(l.language == app.activeLanguage ? .isSelected : [])
            }
            Spacer()
        }
        .padding(16)
        .foregroundStyle(Palette.textPrimary)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(Palette.surfaceRaised.ignoresSafeArea())
    }
}
