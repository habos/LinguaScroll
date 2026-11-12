import Core
import SwiftUI

/// Grouped settings list (UX spec section 5).
struct SettingsView: View {
    @Environment(AppModel.self) private var app
    @Binding var path: [MeRoute]

    var body: some View {
        @Bindable var app = app
        List {
            Section {
                NavigationLink(value: MeRoute.nativeLanguage) {
                    LabeledContent {
                        Text(verbatim: app.user.map { app.language($0.nativeLanguage)?.localizedName() ?? $0.nativeLanguage } ?? "")
                    } label: { Text("Native language") }
                }
            }
            Section {
                Toggle(isOn: $app.captionsOn) { Text("YouTube captions") }
            } footer: {
                Text("Shown by YouTube inside the player, in the clip's language, when the clip has them.")
            }
            Section {
                NavigationLink(value: MeRoute.hiddenChannels) {
                    LabeledContent {
                        Text(verbatim: "\(app.hiddenChannels.all.count)")
                    } label: { Text("Hidden channels") }
                }
                NavigationLink(value: MeRoute.privacy(confirmReset: false)) { Text("About your data") }
            }
            Section {
                Button { path.append(.privacy(confirmReset: true)) } label: {
                    Text("Reset my data").foregroundStyle(Palette.destructive)
                }
            } footer: {
                Text(verbatim: "LinguaScroll \(Self.version) · ") + Text("Clips play in YouTube's player")
            }
        }
        .listRowBackground(Palette.surface)
        .scrollContentBackground(.hidden)
        .background(Palette.base.ignoresSafeArea())
        .navigationTitle(Text("Settings"))
        .navigationBarTitleDisplayMode(.inline)
    }

    /// "{version} ({build})"
    static var version: String {
        let info = Bundle.main.infoDictionary
        let v = info?["CFBundleShortVersionString"] as? String ?? "0"
        let b = info?["CFBundleVersion"] as? String ?? "0"
        return "\(v) (\(b))"
    }
}

/// Same list as onboarding screen 3; saves on tap.
struct NativeLanguageView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var query = ""
    @State private var saving: String?
    @State private var failed = false

    private var options: [LanguageOption] {
        app.nativeOptions
            .filter { query.isEmpty || $0.localizedName().localizedCaseInsensitiveContains(query) || $0.nativeName.localizedCaseInsensitiveContains(query) }
            .sorted { $0.localizedName().localizedStandardCompare($1.localizedName()) == .orderedAscending }
    }

    var body: some View {
        List {
            if failed {
                Text("Couldn't connect. Check your connection and try again.").foregroundStyle(Palette.destructive)
            }
            ForEach(options) { lang in
                Button { save(lang.code) } label: {
                    HStack {
                        Text(lang.localizedName()).foregroundStyle(Palette.textPrimary)
                        Spacer()
                        if saving == lang.code {
                            ProgressView()
                        } else if app.user?.nativeLanguage == lang.code {
                            Image(systemName: "checkmark").foregroundStyle(Palette.accent)
                        }
                    }
                }
                .accessibilityAddTraits(app.user?.nativeLanguage == lang.code ? .isSelected : [])
            }
            .listRowBackground(Palette.surface)
        }
        .searchable(text: $query)
        .scrollContentBackground(.hidden)
        .background(Palette.base.ignoresSafeArea())
        .navigationTitle(Text("Native language"))
        .navigationBarTitleDisplayMode(.inline)
        .task { if app.languages.isEmpty { await app.reloadLanguages() } }
    }

    private func save(_ code: String) {
        guard saving == nil, code != app.user?.nativeLanguage else { return }
        saving = code
        failed = false
        Task {
            do {
                try await app.setNativeLanguage(code)
                dismiss()
            } catch {
                failed = true
            }
            saving = nil
        }
    }
}

/// Channels hidden on this device, each with "Show again" (`DELETE /v1/channel-blocks/{id}`).
struct HiddenChannelsView: View {
    @Environment(AppModel.self) private var app
    @State private var channels: [HiddenChannels.Entry] = []
    @State private var failed = false

    var body: some View {
        List {
            if channels.isEmpty {
                Text("No hidden channels.").foregroundStyle(Palette.textSecondary)
            }
            if failed {
                Text("Couldn't connect. Check your connection and try again.").foregroundStyle(Palette.destructive)
            }
            ForEach(channels) { channel in
                HStack {
                    Text(verbatim: channel.title ?? String(localized: "Unknown channel")).foregroundStyle(Palette.textPrimary)
                    Spacer()
                    Button("Show again") { unhide(channel) }.foregroundStyle(Palette.accent)
                }
                .frame(minHeight: 44)
            }
        }
        .listRowBackground(Palette.surface)
        .scrollContentBackground(.hidden)
        .background(Palette.base.ignoresSafeArea())
        .navigationTitle(Text("Hidden channels"))
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { channels = app.hiddenChannels.all }
    }

    private func unhide(_ channel: HiddenChannels.Entry) {
        failed = false
        let feed = app.deps.feed, id = channel.id
        Task {
            do {
                try await app.deps.session.authorized { try await feed.setChannelBlocked(false, channelID: id, token: $0) }
                app.hiddenChannels.remove(id)
                channels = app.hiddenChannels.all
            } catch {
                failed = true
            }
        }
    }
}

/// "About your data", with the Reset confirm card.
struct PrivacyView: View {
    @Environment(AppModel.self) private var app
    @State private var confirming: Bool
    @State private var erasing = false
    @State private var failed = false

    init(confirming: Bool) {
        _confirming = State(initialValue: confirming)
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    Text("There's no account. When you first opened LinguaScroll, it made a random ID that stays on this phone. It isn't linked to your name, email, Apple ID or advertising ID.")
                    Text("**What we keep with that ID:** your age range, your languages and levels, the clips you like, save, hide or report, the channels you hid, and how long you watch each clip.")
                    Text("**Why:** to pick the next clips for you. We don't sell it or use it for ads.")
                    Text("**YouTube:** clips play in YouTube's own player, so YouTube's privacy policy covers what you watch there.")
                    Text("On a new phone you start fresh. Resetting erases all of it from our server and this phone.")
                    resetCard.id("reset").padding(.top, 8)
                }
                .padding(16)
            }
            .onAppear { if confirming { proxy.scrollTo("reset", anchor: .bottom) } }
        }
        .background(Palette.base.ignoresSafeArea())
        .foregroundStyle(Palette.textPrimary)
        .navigationTitle(Text("About your data"))
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder private var resetCard: some View {
        if confirming {
            VStack(alignment: .leading, spacing: 12) {
                Text("Reset my data?").font(.title3.weight(.semibold))
                Text("Your likes, saves, languages and watch history will be erased. This can't be undone.")
                    .foregroundStyle(Palette.textSecondary)
                if failed {
                    Text("Couldn't connect. Nothing was erased. Try again when you're online.")
                        .font(.footnote).foregroundStyle(Palette.destructive)
                }
                HStack(spacing: 12) {
                    Button("Cancel") { confirming = false; failed = false }
                        .font(.body.weight(.semibold))
                        .frame(maxWidth: .infinity, minHeight: 52)
                        .background(Palette.fill, in: RoundedRectangle(cornerRadius: 14))
                        .buttonStyle(.plain)
                        .disabled(erasing)
                    Button { erase() } label: {
                        Group {
                            if erasing { ProgressView().tint(Palette.base) } else { Text("Erase everything") }
                        }
                        .font(.body.weight(.semibold))
                        .foregroundStyle(Palette.base)
                        .frame(maxWidth: .infinity, minHeight: 52)
                        .background(Palette.destructive, in: RoundedRectangle(cornerRadius: 14))
                    }
                    .buttonStyle(.plain)
                    .disabled(erasing)
                }
            }
            .padding(16)
            .background(Palette.surfaceRaised, in: RoundedRectangle(cornerRadius: 14))
        } else {
            Button { confirming = true } label: {
                Text("Reset my data").foregroundStyle(Palette.destructive).frame(minHeight: 44)
            }
        }
    }

    private func erase() {
        erasing = true
        failed = false
        Task {
            do { try await app.resetData() } catch { failed = true }
            erasing = false
        }
    }
}
