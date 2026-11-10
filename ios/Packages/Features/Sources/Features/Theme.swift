import SwiftUI

/// Colour tokens from docs/ux/v0-ux-spec.md section 1. Brights live around the clip, never on it.
public enum Palette {
    public static let mango = Color(hex: 0xFFC233)        // primary buttons, active tab, links, selection
    public static let bubblegum = Color(hex: 0xFF4F9A)    // Like only
    public static let lagoon = Color(hex: 0x22D3C5)       // Save only, first level card
    public static let grape = Color(hex: 0x6440F5)        // onboarding background, feed language pill
    public static let grapeDeep = Color(hex: 0x4F2FD6)    // rows and cards on grape
    public static let night = Color(hex: 0x15112E)        // Me, Settings, sheets; text on bright fills
    public static let cinema = Color.black                // feed cell and the space around the player
    public static let surface = Color(hex: 0x231D4A)
    public static let surfaceRaised = Color(hex: 0x2D2660)
    public static let fill = Color(hex: 0x3A3275)
    public static let textPrimary = Color(hex: 0xF5F5F7)
    public static let textSecondary = Color(hex: 0xB9B3D9)
    public static let textOnGrape = Color(hex: 0xF1EDFF)
    public static let textTertiary = Color(hex: 0x71717A)
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

/// 52 pt mango button with night text.
struct PrimaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.semibold))
            .foregroundStyle(Palette.night)
            .frame(maxWidth: .infinity, minHeight: 52)
            .background(Palette.mango, in: RoundedRectangle(cornerRadius: 14))
            .opacity(configuration.isPressed ? 0.8 : 1)
    }
}

/// The "LinguaScroll" wordmark, "Scroll" in mango.
struct Wordmark: View {
    var body: some View {
        (Text(verbatim: "Lingua").foregroundStyle(Palette.textPrimary)
            + Text(verbatim: "Scroll").foregroundStyle(Palette.mango))
            .font(.title.weight(.heavy))
            .accessibilityLabel(Text(verbatim: "LinguaScroll"))
    }
}
