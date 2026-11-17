import Core
import SwiftUI

/// One card per language: active one expanded with level chips; others collapsed with Watch.
struct LanguagesView: View {
    @Environment(AppModel.self) private var app
    @Binding var path: [MeRoute]
    @State private var expanded: String?
    @State private var failed = false

    private var learning: [LearningLanguage] { app.user?.learning ?? [] }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(learning, id: \.language) { l in card(l) }
                NavigationLink(value: MeRoute.addLanguage) {
                    HStack {
                        Image(systemName: "plus").foregroundStyle(Palette.accent)
                        Text("Add a language").font(.body.weight(.semibold))
                        Spacer()
                        Image(systemName: "chevron.right").foregroundStyle(Palette.textSecondary)
                    }
                    .padding(.horizontal, 16)
                    .frame(minHeight: 56)
                    .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
                }
                .buttonStyle(.plain)
                if failed {
                    Text("Couldn't connect. Check your connection and try again.").font(.footnote).foregroundStyle(Palette.destructive)
                }
                Text("Your level is a rough guide that steers which clips you see. Nothing is graded. You can't remove your only language.")
                    .font(.footnote).foregroundStyle(Palette.textSecondary).padding(.top, 8)
            }
            .padding(16)
        }
        .background(Palette.base.ignoresSafeArea())
        .foregroundStyle(Palette.textPrimary)
        .navigationTitle(Text("Languages"))
        .navigationBarTitleDisplayMode(.inline)
    }

    private func name(_ code: String) -> String { app.language(code)?.localizedName() ?? code }

    private func card(_ l: LearningLanguage) -> some View {
        let active = l.language == app.activeLanguage
        let open = active || expanded == l.language
        return VStack(alignment: .leading, spacing: 12) {
            HStack {
                Button { withAnimation { expanded = expanded == l.language ? nil : l.language } } label: {
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(verbatim: name(l.language)).font(.body.weight(.semibold))
                            if active {
                                Label { Text("Watching now") } icon: { Image(systemName: "checkmark") }
                                    .font(.footnote).foregroundStyle(Palette.accent)
                            } else {
                                Text(verbatim: "\(Level.cefr(l.level)) · \(Level.plain(l.level))").font(.footnote).foregroundStyle(Palette.textSecondary)
                            }
                        }
                        Spacer()
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .disabled(active)
                if !active {
                    Button("Watch") {
                        app.switchLanguage(to: l.language)
                        app.selectedTab = .feed
                        path.removeAll()
                    }
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Palette.onAccent)
                    .padding(.horizontal, 14)
                    .frame(minHeight: 36)
                    .background(Palette.accent, in: Capsule())
                    .buttonStyle(.plain)
                }
            }

            if open {
                LevelChips(level: l.level) { app.setLevel($0, for: l.language) }
                Text(verbatim: "\(Level.cefr(l.level)) · \(Level.plain(l.level))").font(.footnote).foregroundStyle(Palette.textSecondary)
                if learning.count > 1 {
                    Button(role: .destructive) {
                        Task {
                            failed = false
                            do { try await app.removeLanguage(l.language) } catch { failed = true }
                        }
                    } label: {
                        Text("Remove \(name(l.language))").foregroundStyle(Palette.destructive)
                    }
                    .frame(minHeight: 44)
                }
            }
        }
        .padding(16)
        .background(active ? Palette.accentTint : Palette.surface, in: RoundedRectangle(cornerRadius: 14))
    }
}

/// A1 to C2 as a radio group; selected = accent.
struct LevelChips: View {
    let level: Int
    let pick: (Int) -> Void

    var body: some View {
        HStack(spacing: 6) {
            ForEach(1...6, id: \.self) { n in
                Button { pick(n) } label: {
                    Text(verbatim: Level.cefr(n))
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(n == level ? Palette.onAccent : Palette.textPrimary)
                        .frame(maxWidth: .infinity, minHeight: 44)
                        .background(n == level ? Palette.accent : Palette.fill, in: RoundedRectangle(cornerRadius: 10))
                }
                .buttonStyle(.plain)
                .accessibilityLabel(Text(verbatim: "\(Level.cefr(n)), \(Level.plain(n))"))
                .accessibilityAddTraits(n == level ? .isSelected : [])
            }
        }
    }
}

/// Languages not added yet, then the onboarding level cards; back to Languages with it active.
struct AddLanguageView: View {
    @Environment(AppModel.self) private var app
    @Binding var path: [MeRoute]
    @State private var picked: String?
    @State private var adding: Int?
    @State private var failed = false

    private var options: [LanguageOption] {
        let have = Set((app.user?.learning ?? []).map(\.language))
        return app.learnable.filter { !have.contains($0.code) }
    }

    var body: some View {
        Group {
            if let picked {
                LevelStep(language: picked, registering: adding, failed: failed) { level in add(picked, level) }
            } else {
                ScrollView {
                    VStack(spacing: 8) {
                        if app.learnable.isEmpty { ProgressView().tint(Palette.textPrimary).frame(minHeight: 120) }
                        ForEach(options) { lang in
                            Button { picked = lang.code } label: {
                                HStack {
                                    Text(lang.localizedName()).font(.body.weight(.semibold))
                                    Spacer()
                                    Text(verbatim: lang.nativeName).foregroundStyle(Palette.textSecondary)
                                }
                                .padding(.horizontal, 16)
                                .frame(minHeight: 56)
                                .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(16)
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(Palette.base.ignoresSafeArea())
        .foregroundStyle(Palette.textPrimary)
        .navigationTitle(Text("Add a language"))
        .navigationBarTitleDisplayMode(.inline)
        .task { if app.languages.isEmpty { await app.reloadLanguages() } }
    }

    private func add(_ code: String, _ level: Int) {
        adding = level
        failed = false
        Task {
            do {
                try await app.addLanguage(code, level: level)
                if path.last == .addLanguage { path.removeLast() }
            } catch {
                failed = true
            }
            adding = nil
        }
    }
}
