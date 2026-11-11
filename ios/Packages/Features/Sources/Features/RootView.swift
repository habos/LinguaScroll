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
    var body: some View {
        TabView {
            FeedPlaceholder()
                .tabItem { Label("Feed", systemImage: "play.rectangle.fill") }
            MePlaceholder()
                .tabItem { Label("Me", systemImage: "person.crop.circle") }
        }
        .toolbarBackground(Palette.cinema, for: .tabBar)
        .toolbarBackground(.visible, for: .tabBar)
    }
}

/// Stand-in until the V6 feed lands: shows which language the feed will play.
struct FeedPlaceholder: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                LanguagePill(code: model.activeLanguage)
                Spacer()
            }
            .frame(height: 44)
            .padding(.horizontal, 16)
            Spacer()
            VStack(spacing: 12) {
                ProgressView().tint(Palette.textPrimary)
                Text("Finding \(languageName) clips…").foregroundStyle(Palette.textSecondary)
            }
            Spacer()
        }
        .foregroundStyle(Palette.textPrimary)
        .background(Palette.cinema.ignoresSafeArea())
    }

    private var languageName: String {
        model.language(model.activeLanguage)?.localizedName() ?? model.activeLanguage
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
        .frame(height: 34)
        .background(Palette.accentTint, in: Capsule())
    }
}

/// Stand-in until V7: Me with the user's languages.
struct MePlaceholder: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        NavigationStack {
            List {
                Section {
                    ForEach(model.user?.learning ?? [], id: \.language) { l in
                        HStack {
                            Text(model.language(l.language)?.localizedName() ?? l.language)
                            Spacer()
                            Text(verbatim: "\(Level.cefr(l.level)) · \(Level.plain(l.level))").foregroundStyle(Palette.textSecondary)
                        }
                    }
                } header: {
                    Text("Learning")
                }
                .listRowBackground(Palette.surface)
            }
            .scrollContentBackground(.hidden)
            .background(Palette.base.ignoresSafeArea())
            .navigationTitle(Text("Me"))
        }
    }
}
