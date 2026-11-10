import Foundation
import HTTPTypes
import OpenAPIRuntime
import OpenAPIURLSession

/// Builds the generated client for a base URL (debug builds use
/// http://localhost:8080). Pass a token for the authenticated routes.
public enum LinguaScrollAPI {
    public static func client(baseURL: URL, token: String? = nil) -> Client {
        Client(
            serverURL: baseURL,
            transport: URLSessionTransport(),
            middlewares: token.map { [BearerTokenMiddleware(token: $0)] } ?? []
        )
    }
}

/// Adds `Authorization: Bearer <token>` to every request.
public struct BearerTokenMiddleware: ClientMiddleware {
    let token: String

    public init(token: String) { self.token = token }

    public func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var request = request
        request.headerFields[.authorization] = "Bearer \(token)"
        return try await next(request, body, baseURL)
    }
}
