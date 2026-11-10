import Foundation
import OpenAPIRuntime
import OpenAPIURLSession

/// Builds the generated client for a base URL (debug builds use
/// http://localhost:8080). The bearer-token middleware arrives with V5.
public enum LinguaScrollAPI {
    public static func client(baseURL: URL) -> Client {
        Client(serverURL: baseURL, transport: URLSessionTransport())
    }
}
