import Core
import SwiftUI

/// A Me list played in the feed layout from the tapped clip; Back returns to the grid.
struct LibraryPlayerScreen: View {
    @Environment(\.dismiss) private var dismiss
    @State private var model: FeedModel

    init(model: FeedModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Button { dismiss() } label: {
                    Label { Text("Back") } icon: { Image(systemName: "chevron.left") }
                        .font(.body.weight(.semibold))
                        .frame(minHeight: 44)
                }
                .buttonStyle(.plain)
                Spacer()
                Text(model.source == .library(.saved) ? "Saved" : "Liked")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Palette.textSecondary)
            }
            .frame(height: 44)
            .padding(.horizontal, 16)

            if model.cards.isEmpty {
                Spacer()
            } else {
                FeedContainer(model: model).frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(Palette.cinema.ignoresSafeArea(edges: .top))
        .foregroundStyle(Palette.textPrimary)
        .toolbar(.hidden, for: .navigationBar)
    }
}
