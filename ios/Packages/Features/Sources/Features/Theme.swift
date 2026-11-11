import SwiftUI

/// Colour tokens from docs/ux/v0-ux-spec.md section 1 (single-accent "Lagoon" scheme).
/// Like and save share the accent and differ by icon shape.
public enum Palette {
    public static let accent = Color(hex: 0x22D3C5)        // buttons, links, active tab, liked, saved, selected
    public static let onAccent = Color(hex: 0x0E1416)      // text on accent
    public static let accentTint = Color(hex: 0x10363A)    // language pill, selected cards
    public static let base = Color(hex: 0x0E1416)          // every non-feed screen, onboarding included
    public static let cinema = Color.black                 // feed cell and the space around the player
    public static let surface = Color(hex: 0x1A2326)
    public static let surfaceRaised = Color(hex: 0x222D31)
    public static let fill = Color(hex: 0x2A373B)
    public static let textPrimary = Color(hex: 0xF5F5F7)
    public static let textSecondary = Color(hex: 0x9FB3B5)
    public static let destructive = Color(hex: 0xFF7A66)
}

extension Color {
    init(hex: UInt32) {
        self.init(
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255
        )
    }
}

/// 52 pt accent button.
struct PrimaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.semibold))
            .foregroundStyle(Palette.onAccent)
            .frame(maxWidth: .infinity, minHeight: 52)
            .background(Palette.accent, in: RoundedRectangle(cornerRadius: 14))
            .opacity(configuration.isPressed ? 0.8 : 1)
    }
}

/// The "LinguaScroll" wordmark, "Scroll" in the accent.
struct Wordmark: View {
    var body: some View {
        (Text(verbatim: "Lingua").foregroundStyle(Palette.textPrimary)
            + Text(verbatim: "Scroll").foregroundStyle(Palette.accent))
            .font(.title.weight(.heavy))
            .accessibilityLabel(Text(verbatim: "LinguaScroll"))
    }
}
