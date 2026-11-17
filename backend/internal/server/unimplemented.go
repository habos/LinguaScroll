// Stubs for operations in api/openapi.yaml that no module implements yet.
// When a module implements an endpoint, delete its stub here and add the
// real method to Server. Every v0 endpoint is implemented as of V4.

package server

// Unimplemented answers an operation with 501 not_implemented. Server
// embeds it, so a new route only needs a real method once its module lands.
type Unimplemented struct{}
