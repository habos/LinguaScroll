// swift-tools-version: 6.0
// SwiftUI screens and app state. Talks to the server only through Core's protocols.
import PackageDescription

let package = Package(
    name: "Features",
    platforms: [.iOS(.v17)],
    products: [.library(name: "Features", targets: ["Features"])],
    dependencies: [.package(path: "../Core")],
    targets: [
        .target(name: "Features", dependencies: ["Core"]),
    ]
)
