import Core
import SwiftUI

enum MeRoute: Hashable {
    case settings, languages, addLanguage, nativeLanguage, hiddenChannels
    case privacy(confirmReset: Bool)
    case play(LibraryKind, index: Int)
}

/// Me tab: languages card, Saved | Liked grid, settings behind the gear.
struct MeView: View {
    @Environment(AppModel.self) private var app
    @State private var path: [MeRoute] = []
    @State private var kind: LibraryKind = .saved
    @State private var saved: LibraryModel
    @State private var liked: LibraryModel

    init(deps: AppDependencies) {
        _saved = State(initialValue: LibraryModel(kind: .saved, deps: deps))
        _liked = State(initialValue: LibraryModel(kind: .liked, deps: deps))
    }

    private var list: LibraryModel { kind == .saved ? saved : liked }

    var body: some View {
        NavigationStack(path: $path) {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    learningCard.padding(.horizontal, 16)
                    Picker(selection: $kind) {
                        Text("Saved").tag(LibraryKind.saved)
                        Text("Liked").tag(LibraryKind.liked)
                    } label: { EmptyView() }
                        .pickerStyle(.segmented)
                        .padding(.horizontal, 16)
                    LibraryGrid(model: list, path: $path)
                }
                .padding(.vertical, 8)
            }
            .background(Palette.base.ignoresSafeArea())
            .foregroundStyle(Palette.textPrimary)
            .navigationTitle(Text("Me"))
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    NavigationLink(value: MeRoute.settings) {
                        Image(systemName: "gearshape").accessibilityLabel(Text("Settings"))
                    }
                }
            }
            .navigationDestination(for: MeRoute.self) { route in destination(route) }
        }
        .task(id: kind) { await list.reload() }
        .onAppear { Task { await list.reload() } }
    }

    private var learningCard: some View {
        NavigationLink(value: MeRoute.languages) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Learning").font(.footnote).foregroundStyle(Palette.textSecondary)
                    Text(verbatim: learningSummary).font(.body.weight(.semibold)).lineLimit(2)
                }
                Spacer()
                Image(systemName: "chevron.right").foregroundStyle(Palette.textSecondary)
            }
            .padding(16)
            .frame(minHeight: 64)
            .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
        }
        .buttonStyle(.plain)
    }

    /// "Spanish B1 · Japanese A1"
    private var learningSummary: String {
        (app.user?.learning ?? []).map { l in
            "\(app.language(l.language)?.localizedName() ?? l.language) \(Level.cefr(l.level))"
        }.joined(separator: " · ")
    }

    @ViewBuilder private func destination(_ route: MeRoute) -> some View {
        switch route {
        case .settings: SettingsView(path: $path)
        case .languages: LanguagesView(path: $path)
        case .addLanguage: AddLanguageView(path: $path)
        case .nativeLanguage: NativeLanguageView()
        case .hiddenChannels: HiddenChannelsView()
        case .privacy(let confirm): PrivacyView(confirming: confirm)
        case .play(let kind, let index):
            let list = kind == .saved ? saved : liked
            LibraryPlayerScreen(model: FeedModel(library: kind, cards: list.cards, cursor: list.cursor, startIndex: index,
                                                 deps: app.deps, captionsOn: app.captionsOn))
        }
    }
}

/// 3 columns, 2 pt gaps, 9:16 cells; empty and error states in its place.
private struct LibraryGrid: View {
    @Environment(AppModel.self) private var app
    let model: LibraryModel
    @Binding var path: [MeRoute]

    private let columns = Array(repeating: GridItem(.flexible(), spacing: 2), count: 3)

    var body: some View {
        switch model.state {
        case .loading where model.cards.isEmpty:
            ProgressView().tint(Palette.textPrimary).frame(maxWidth: .infinity, minHeight: 160)
        case .failed:
            VStack(spacing: 12) {
                Text("Couldn't load your clips").font(.body.weight(.semibold))
                Button("Try again") { Task { await model.reload() } }
                    .buttonStyle(PrimaryButtonStyle()).frame(maxWidth: 220)
            }
            .frame(maxWidth: .infinity, minHeight: 200)
        default:
            if model.cards.isEmpty { empty } else { grid }
        }
    }

    private var grid: some View {
        LazyVGrid(columns: columns, spacing: 2) {
            ForEach(Array(model.cards.enumerated()), id: \.element.id) { index, card in
                Button { path.append(.play(model.kind, index: index)) } label: { cell(card) }
                    .buttonStyle(.plain)
                    .accessibilityLabel(Text(verbatim: card.title))
                    .contextMenu {
                        Button(role: .destructive) { model.remove(card) } label: {
                            model.kind == .saved
                                ? Label { Text("Unsave") } icon: { Image(systemName: "bookmark.slash") }
                                : Label { Text("Unlike") } icon: { Image(systemName: "heart.slash") }
                        }
                        ShareLink(item: card.shareURL) { Label { Text("Share") } icon: { Image(systemName: "square.and.arrow.up") } }
                    }
                    .task { await model.loadMoreIfNeeded(after: card) }
            }
        }
    }

    private func cell(_ card: VideoCard) -> some View {
        Color.clear
            .aspectRatio(9.0 / 16.0, contentMode: .fit)
            .overlay { ThumbnailImage(url: card.thumbnailURL) }
            .overlay(alignment: .bottomLeading) { chip(Self.duration(card.durationMs)).padding(4) }
            .overlay(alignment: .topTrailing) {
                if (app.user?.learning.count ?? 0) > 1 { chip(card.language.uppercased()).padding(4) }
            }
            .clipped()
            .contentShape(Rectangle())
    }

    private func chip(_ text: String) -> some View {
        Text(verbatim: text)
            .font(.caption2.weight(.semibold).monospacedDigit())
            .foregroundStyle(.white)
            .padding(.horizontal, 5)
            .padding(.vertical, 2)
            .background(Color.black.opacity(0.55), in: RoundedRectangle(cornerRadius: 4))
    }

    static func duration(_ ms: Int) -> String {
        let s = max(0, ms / 1000)
        return String(format: "%d:%02d", s / 60, s % 60)
    }

    private var empty: some View {
        VStack(spacing: 8) {
            Image(systemName: model.kind == .saved ? "bookmark" : "heart").font(.largeTitle).foregroundStyle(Palette.textSecondary)
            if model.kind == .saved {
                Text("Nothing saved yet").font(.body.weight(.semibold))
                Text("Tap the bookmark under a clip to keep it here.").foregroundStyle(Palette.textSecondary)
            } else {
                Text("No likes yet").font(.body.weight(.semibold))
                Text("Clips you like show up here. Likes also tune your feed.").foregroundStyle(Palette.textSecondary)
            }
        }
        .multilineTextAlignment(.center)
        .padding(32)
        .frame(maxWidth: .infinity)
    }
}
