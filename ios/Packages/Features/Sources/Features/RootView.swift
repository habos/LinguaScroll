import Core
import SwiftUI

/// App root: block screen, onboarding, or the two tabs.
public struct RootView: View {
    @State private var model: AppModel

    public init(model: AppModel) {
        _model = State(initialValue: model)
    }

    public var body: some View {
        Group {
            switch model.phase {
            case .launching:
                Palette.base.ignoresSafeArea()
            case .blocked:
                BlockedView()
            case .onboarding:
                OnboardingView()
            case .ready:
                MainTabs()
            }
        }
        .environment(model)
        .preferredColorScheme(.dark)
        .tint(Palette.accent)
        .task { await model.launch() }
    }
}

/// Under-13: calm, no buttons, no hint of the cut-off, no way back.
struct BlockedView: View {
    var body: some View {
        VStack(spacing: 16) {
            Wordmark()
            Text("LinguaScroll is for ages 13 and up").font(.title3.weight(.semibold)).multilineTextAlignment(.center)
            Text("Thanks for stopping by.").foregroundStyle(Palette.textSecondary)
        }
        .foregroundStyle(Palette.textPrimary)
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Palette.base.ignoresSafeArea())
    }
}

struct MainTabs: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        @Bindable var app = app
        TabView(selection: $app.selectedTab) {
            FeedScreen()
                .tabItem { Label("Feed", systemImage: "play.rectangle.fill") }
                .tag(AppModel.Tab.feed)
            MeView(deps: app.deps)
                .tabItem { Label("Me", systemImage: "person.crop.circle") }
                .tag(AppModel.Tab.me)
        }
        .toolbarBackground(Palette.cinema, for: .tabBar)
        .toolbarBackground(.visible, for: .tabBar)
    }
}

/// "{Language} · {CEFR}" pill on the accent tint (tap opens the switcher in V6).
struct LanguagePill: View {
    @Environment(AppModel.self) private var model
    let code: String

    private var level: Int? { model.user?.learning.first { $0.language == code }?.level }

    var body: some View {
        HStack(spacing: 4) {
            Text(model.language(code)?.localizedName() ?? code).font(.subheadline.weight(.semibold))
            if let level {
                Text(verbatim: "· \(Level.cefr(level))").font(.subheadline).foregroundStyle(Palette.textSecondary)
            }
            Image(systemName: "chevron.down").font(.caption.weight(.semibold))
        }
        .foregroundStyle(Palette.textPrimary)
        .padding(.horizontal, 14)
        .frame(minHeight: 44)
        .frame(height: 34)
        .background(Palette.accentTint, in: Capsule())
    }
}
