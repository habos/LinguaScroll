import AVFoundation
import SwiftUI

@main
struct SpikeApp: App {
    init() {
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        try? AVAudioSession.sharedInstance().setActive(true)
    }

    var body: some Scene {
        WindowGroup { SpikeRoot().preferredColorScheme(.dark) }
    }
}

struct SpikeRoot: View {
    @AppStorage("spike.ids") private var idsText = ""
    @AppStorage("spike.simultaneous") private var simultaneous = true
    @AppStorage("spike.controls") private var controls = true
    @StateObject private var stats = SpikeStats()
    @State private var editing = false

    private var ids: [String] { VideoIDParser.parse(idsText) }

    var body: some View {
        VStack(spacing: 0) {
            topBar
            if ids.isEmpty {
                ContentUnavailableView {
                    Label("Paste some Shorts", systemImage: "play.rectangle.on.rectangle")
                } description: {
                    Text("In the YouTube app, Share a Short, Copy link, then paste the links here (one per line). 10 or more is best.")
                } actions: {
                    Button("Add links") { editing = true }.buttonStyle(.borderedProminent)
                }
            } else {
                FeedView(ids: ids, simultaneous: simultaneous, controls: controls, stats: stats)
                    .id("\(simultaneous)-\(controls)-\(ids.joined())") // rebuild on change
            }
        }
        .background(Color.black)
        .sheet(isPresented: $editing) { IDEditor(text: $idsText) }
    }

    private var topBar: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Toggle("Swipe", isOn: $simultaneous).toggleStyle(.switch).fixedSize()
                Toggle("YT controls", isOn: $controls).toggleStyle(.switch).fixedSize()
                Spacer()
                Button("Links") { editing = true }
            }
            Text(hud).font(.caption2.monospaced()).foregroundStyle(.secondary).lineLimit(2)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 6)
        .background(Color(white: 0.08))
    }

    private var hud: String {
        let ttff = stats.lastTTFF.map { "\($0)ms" } ?? "-"
        let median = stats.medianTTFF.map { "\($0)ms" } ?? "-"
        return "#\(stats.index + 1) \(stats.state)  ttff \(ttff) med \(median)  swipes \(stats.swipes)  2xAudio \(stats.doubleAudioEvents)\n\(stats.lastEvent)"
    }
}

struct FeedView: UIViewControllerRepresentable {
    let ids: [String]
    let simultaneous: Bool
    let controls: Bool
    let stats: SpikeStats

    func makeUIViewController(context: Context) -> FeedViewController {
        FeedViewController(ids: ids, simultaneous: simultaneous, controls: controls, stats: stats)
    }

    func updateUIViewController(_ vc: FeedViewController, context: Context) {}
}

struct IDEditor: View {
    @Binding var text: String
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            TextEditor(text: $text)
                .font(.footnote.monospaced())
                .padding()
                .navigationTitle("Shorts links (\(VideoIDParser.parse(text).count))")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Paste") { if let s = UIPasteboard.general.string { text += (text.isEmpty ? "" : "\n") + s } }
                    }
                    ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
                }
        }
    }
}

/// Accepts bare 11-char IDs, youtube.com/shorts/ID, youtu.be/ID and watch?v=ID.
enum VideoIDParser {
    static func parse(_ text: String) -> [String] {
        let pattern = #"(?:shorts/|youtu\.be/|[?&]v=|^)([A-Za-z0-9_-]{11})(?![A-Za-z0-9_-])"#
        guard let re = try? NSRegularExpression(pattern: pattern, options: [.anchorsMatchLines]) else { return [] }
        var seen = Set<String>()
        var out: [String] = []
        for line in text.split(whereSeparator: \.isNewline) {
            let s = String(line).trimmingCharacters(in: .whitespaces)
            let range = NSRange(s.startIndex..., in: s)
            if let m = re.firstMatch(in: s, range: range), let r = Range(m.range(at: 1), in: s) {
                let id = String(s[r])
                if seen.insert(id).inserted { out.append(id) }
            }
        }
        return out
    }
}
