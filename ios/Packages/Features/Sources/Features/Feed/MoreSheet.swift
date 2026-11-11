import Core
import SwiftUI

/// More options for the current clip: hide, not interested in the channel, report.
struct MoreSheet: View {
    enum Action { case hide, blockChannel, report(ReportReason), cancel }

    let card: VideoCard
    let languageName: String
    let done: (Action) -> Void
    @State private var reporting = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if reporting {
                Text("Report this clip").font(.title3.weight(.semibold)).padding(16)
                reason(.notThisLanguage, String(localized: "Not in \(languageName)"))
                reason(.inappropriate, String(localized: "Not appropriate"))
                reason(.spam, String(localized: "Spam or misleading"))
                reason(.other, String(localized: "Something else"))
            } else {
                row("eye.slash", String(localized: "Hide this clip"), String(localized: "You won't see it again")) { done(.hide) }
                row("nosign", String(localized: "Not interested in \(card.channelTitle)"),
                    String(localized: "Hide clips from this channel. Undo in Settings")) { done(.blockChannel) }
                row("flag", String(localized: "Report"), String(localized: "Not in \(languageName), not appropriate, spam, other"),
                    destructive: true) { reporting = true }
            }
            Button { done(.cancel) } label: {
                Text("Cancel").font(.body.weight(.semibold)).frame(maxWidth: .infinity, minHeight: 52)
            }
            .foregroundStyle(Palette.textPrimary)
            .background(Palette.fill, in: RoundedRectangle(cornerRadius: 14))
            .padding(16)
            Spacer(minLength: 0)
        }
        .foregroundStyle(Palette.textPrimary)
        .background(Palette.surfaceRaised.ignoresSafeArea())
        .preferredColorScheme(.dark)
    }

    private func row(_ symbol: String, _ title: String, _ detail: String, destructive: Bool = false, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 14) {
                Image(systemName: symbol).frame(width: 28)
                VStack(alignment: .leading, spacing: 2) {
                    Text(verbatim: title).font(.body.weight(.semibold))
                    Text(verbatim: detail).font(.footnote).foregroundStyle(Palette.textSecondary)
                }
                Spacer()
            }
            .foregroundStyle(destructive ? Palette.destructive : Palette.textPrimary)
            .padding(.horizontal, 16)
            .frame(minHeight: 60)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private func reason(_ r: ReportReason, _ title: String) -> some View {
        Button { done(.report(r)) } label: {
            Text(verbatim: title).frame(maxWidth: .infinity, minHeight: 52, alignment: .leading).padding(.horizontal, 16).contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}
