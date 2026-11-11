import Core
import SwiftUI

/// Four screens: birth year, language to learn, native language, level.
/// Nothing is sent until the level is tapped.
struct OnboardingView: View {
    @Environment(AppModel.self) private var model
    @State private var step = 1
    @State private var birthYear = Calendar.current.component(.year, from: Date()) - 20
    @State private var learning: String?
    @State private var native: String?
    @State private var registeringLevel: Int?
    @State private var failed = false

    var body: some View {
        VStack(spacing: 0) {
            topBar
            Group {
                switch step {
                case 1: BirthYearStep(year: $birthYear) { step = 2 }
                case 2: LearnStep(selection: $learning) { step = 3 }
                case 3: NativeStep(learning: learning, selection: $native) { step = 4 }
                default: LevelStep(language: learning, registering: registeringLevel, failed: failed, pick: register)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }
        .background(Palette.base.ignoresSafeArea())
        .foregroundStyle(Palette.textPrimary)
        .animation(.default, value: step)
    }

    private var topBar: some View {
        HStack {
            if step > 1 {
                Button { step -= 1 } label: {
                    Image(systemName: "chevron.left").frame(width: 44, height: 44)
                }
                .accessibilityLabel(Text("Back"))
                .disabled(registeringLevel != nil)
            }
            Spacer()
            Text("\(step) of 4").font(.footnote).foregroundStyle(Palette.textSecondary)
        }
        .frame(height: 44)
        .padding(.horizontal, 16)
    }

    private func register(level: Int) {
        guard let learning, let native, registeringLevel == nil else { return }
        registeringLevel = level
        failed = false
        let answers = OnboardingAnswers(birthYear: birthYear, learning: learning, native: native, level: level)
        Task {
            do {
                try await model.register(answers)
            } catch APIError.ageBlocked {
                // AppModel already switched to the block screen.
            } catch {
                failed = true
            }
            registeringLevel = nil
        }
    }
}

// MARK: - Step 1

private struct BirthYearStep: View {
    @Binding var year: Int
    let next: () -> Void
    private let thisYear = Calendar.current.component(.year, from: Date())

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Wordmark()
            Text("Learn a language by scrolling.").font(.title3).foregroundStyle(Palette.textSecondary)
            Text("First, what year were you born?").font(.largeTitle.bold())
            Picker(selection: $year) {
                ForEach(Array((thisYear - 100...thisYear).reversed()), id: \.self) { y in
                    Text(verbatim: String(y)).tag(y)
                }
            } label: {
                Text("Birth year")
            }
            .pickerStyle(.wheel)
            .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
            Text("We use this only to check your age. We keep an age range, not the year.")
                .font(.footnote).foregroundStyle(Palette.textSecondary)
            Spacer()
            Button("Continue", action: next).buttonStyle(PrimaryButtonStyle())
        }
        .padding(24)
    }
}

// MARK: - Step 2

private struct LearnStep: View {
    @Environment(AppModel.self) private var model
    @Binding var selection: String?
    let next: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 8) {
                Text("Which language do you want to learn?").font(.largeTitle.bold()).padding(.bottom, 8)
                if model.learnable.isEmpty {
                    LanguagesLoading()
                }
                ForEach(model.learnable) { lang in
                    Button {
                        selection = lang.code
                        next()
                    } label: {
                        HStack {
                            Text(lang.localizedName()).font(.body.weight(.semibold))
                            Spacer()
                            Text(verbatim: lang.nativeName).foregroundStyle(Palette.textSecondary)
                        }
                        .padding(.horizontal, 16)
                        .frame(minHeight: 56)
                        .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
                        .overlay {
                            if selection == lang.code {
                                RoundedRectangle(cornerRadius: 14).stroke(Palette.accent, lineWidth: 2)
                            }
                        }
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(24)
        }
    }
}

/// Spinner, or a retry when `GET /v1/languages` failed.
private struct LanguagesLoading: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        if model.languagesFailed {
            VStack(alignment: .leading, spacing: 12) {
                Text("Couldn't connect. Check your connection and tap again.").foregroundStyle(Palette.destructive)
                Button("Try again") { Task { await model.reloadLanguages() } }.buttonStyle(PrimaryButtonStyle())
            }
        } else {
            ProgressView().tint(Palette.textPrimary).frame(maxWidth: .infinity, minHeight: 120)
        }
    }
}

// MARK: - Step 3

private struct NativeStep: View {
    @Environment(AppModel.self) private var model
    let learning: String?
    @Binding var selection: String?
    let next: () -> Void
    @State private var query = ""

    private var deviceLanguage: String? { Locale.current.language.languageCode?.identifier }

    private var options: [LanguageOption] {
        model.nativeOptions
            .filter { $0.code != learning }
            .filter { query.isEmpty || $0.localizedName().localizedCaseInsensitiveContains(query) || $0.nativeName.localizedCaseInsensitiveContains(query) }
            .sorted { lhs, rhs in
                if lhs.code == deviceLanguage { return true }
                if rhs.code == deviceLanguage { return false }
                return lhs.localizedName().localizedStandardCompare(rhs.localizedName()) == .orderedAscending
            }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("What's your native language?").font(.largeTitle.bold())
            Text("We picked it from your phone's settings.").font(.footnote).foregroundStyle(Palette.textSecondary)
            TextField(text: $query) { Text("Search") }
                .padding(.horizontal, 12)
                .frame(height: 44)
                .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
            ScrollView {
                LazyVStack(spacing: 8) {
                    if model.nativeOptions.isEmpty { LanguagesLoading() }
                    ForEach(options) { lang in
                        Button { selection = lang.code } label: {
                            HStack {
                                Text(lang.localizedName())
                                Spacer()
                                if selection == lang.code { Image(systemName: "checkmark").foregroundStyle(Palette.accent) }
                            }
                            .padding(.horizontal, 16)
                            .frame(minHeight: 52)
                            .background(Palette.surface, in: RoundedRectangle(cornerRadius: 14))
                            .overlay {
                                if selection == lang.code {
                                    RoundedRectangle(cornerRadius: 14).stroke(Palette.accent, lineWidth: 2)
                                }
                            }
                        }
                        .buttonStyle(.plain)
                        .accessibilityAddTraits(selection == lang.code ? .isSelected : [])
                    }
                }
            }
            Button("Continue", action: next)
                .buttonStyle(PrimaryButtonStyle())
                .disabled(selection == nil)
                .opacity(selection == nil ? 0.5 : 1)
        }
        .padding(24)
        .onAppear {
            if selection == nil, let d = deviceLanguage, d != learning,
               model.nativeOptions.contains(where: { $0.code == d }) {
                selection = d
            }
        }
    }
}

// MARK: - Step 4

private struct LevelStep: View {
    @Environment(AppModel.self) private var model
    let language: String?
    let registering: Int?
    let failed: Bool
    let pick: (Int) -> Void

    private var languageName: String {
        language.flatMap { model.language($0)?.localizedName() } ?? ""
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("How much \(languageName) do you know?").font(.largeTitle.bold()).padding(.bottom, 8)
            card(level: 1, title: "Just starting", detail: "I know a few words and phrases.")
            card(level: 3, title: "Can get by", detail: "I can follow slow, simple conversations.")
            card(level: 5, title: "Pretty comfortable", detail: "I understand most everyday talk.")
            if failed {
                Text("Couldn't connect. Check your connection and tap again.")
                    .font(.footnote).foregroundStyle(Palette.destructive)
            }
            Text("A rough guess is fine. It only steers which clips you see first, and you can change it in Me.")
                .font(.footnote).foregroundStyle(Palette.textSecondary)
            Spacer()
        }
        .padding(24)
    }

    private func card(level: Int, title: LocalizedStringKey, detail: LocalizedStringKey) -> some View {
        Button { pick(level) } label: {
            HStack(alignment: .center) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(title).font(.body.weight(.semibold))
                    Text(detail).font(.subheadline).foregroundStyle(Palette.textSecondary)
                }
                Spacer()
                if registering == level { ProgressView().tint(Palette.accent) }
            }
            .foregroundStyle(Palette.textPrimary)
            .padding(16)
            .frame(maxWidth: .infinity, minHeight: 72, alignment: .leading)
            .background(registering == level ? Palette.accentTint : Palette.surface, in: RoundedRectangle(cornerRadius: 14))
        }
        .buttonStyle(.plain)
        .disabled(registering != nil)
        .opacity(registering != nil && registering != level ? 0.5 : 1)
    }
}
