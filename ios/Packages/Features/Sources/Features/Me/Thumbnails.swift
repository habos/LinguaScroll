import SwiftUI
import UIKit

/// YouTube thumbnails are fetched from YouTube's public URL each time and held in memory only:
/// nothing is written to disk as stored data (QA V7-06, YouTube's storage rules).
enum Thumbnails {
    static let session: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.urlCache = URLCache(memoryCapacity: 32 * 1024 * 1024, diskCapacity: 0)
        return URLSession(configuration: config)
    }()
}

/// A thumbnail cropped to fill its frame, loaded through `Thumbnails.session`.
struct ThumbnailImage: View {
    let url: URL
    @State private var image: UIImage?

    var body: some View {
        ZStack {
            Palette.surface
            if let image {
                Image(uiImage: image).resizable().scaledToFill()
            }
        }
        .clipped()
        .task(id: url) {
            image = nil
            guard let (data, _) = try? await Thumbnails.session.data(from: url) else { return }
            image = UIImage(data: data)
        }
    }
}
