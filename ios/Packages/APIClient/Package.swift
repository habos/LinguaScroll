// swift-tools-version: 5.10
// Generated client for api/openapi.yaml. `make gen` copies the spec into
// Sources/APIClient; the swift-openapi-generator build plugin does the rest.
// The generator is pinned to one minor version: the set of files it writes
// changes between releases (1.14 splits Types.swift into Types+*.swift), and
// a mismatch with a stale DerivedData shows up as "Build input files cannot
// be found". Bump it on purpose, then delete DerivedData once.
import PackageDescription

let package = Package(
    name: "APIClient",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "APIClient", targets: ["APIClient"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-openapi-generator", .upToNextMinor(from: "1.14.4")),
        .package(url: "https://github.com/apple/swift-openapi-runtime", from: "1.8.0"),
        .package(url: "https://github.com/apple/swift-openapi-urlsession", from: "1.1.0"),
        .package(url: "https://github.com/apple/swift-http-types", from: "1.0.0"),
    ],
    targets: [
        .target(
            name: "APIClient",
            dependencies: [
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
                .product(name: "HTTPTypes", package: "swift-http-types"),
            ],
            plugins: [
                .plugin(name: "OpenAPIGenerator", package: "swift-openapi-generator"),
            ]
        ),
    ]
)
