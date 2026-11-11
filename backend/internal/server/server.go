// Package server wires the generated OpenAPI handler to the module
// implementations and the shared middleware.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/habos/linguascroll/backend/internal/api"
	"github.com/habos/linguascroll/backend/internal/devices"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AddressLimitFactor scales the per-user rate limit into the per-address
// limit applied before authentication.
const AddressLimitFactor = 5

// ErrNotImplemented marks an endpoint whose module hasn't landed yet.
var ErrNotImplemented = errors.New("not implemented")

// Deps is what the server needs from main.
type Deps struct {
	DB                 *pgxpool.Pool
	Log                *slog.Logger
	Tokens             httpx.TokenLookup
	RateLimitPerMinute int
}

// Server implements api.StrictServerInterface. Endpoints not yet built
// fall through to Unimplemented (501).
type Server struct {
	Unimplemented
	db      *pgxpool.Pool
	devices *devices.Service
}

var _ api.StrictServerInterface = (*Server)(nil)

// publicRoutes need no bearer token.
func publicRoute(r *http.Request) bool {
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/healthz" || r.URL.Path == "/v1/config" || r.URL.Path == "/v1/languages"):
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/anonymous":
		return true
	}
	return false
}

// New returns the full HTTP handler.
func New(d Deps) http.Handler {
	s := &Server{db: d.DB, devices: &devices.Service{DB: d.DB}}
	strict := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, ErrNotImplemented) {
				httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "this endpoint is not built yet")
				return
			}
			var apiErr *httpx.APIError
			if errors.As(err, &apiErr) {
				httpx.WriteError(w, apiErr.Status, apiErr.Code, apiErr.Message)
				return
			}
			d.Log.ErrorContext(r.Context(), "handler error", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		},
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "no such route")
	})
	h := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
	})
	return httpx.Chain(h,
		httpx.RequestID,
		httpx.Recover(d.Log),
		httpx.AccessLog(d.Log),
		// Per address first (generous, so several phones behind one NAT are
		// fine), so invented tokens can't dodge the limit; then per user.
		httpx.NewRateLimiter(d.RateLimitPerMinute*AddressLimitFactor).ByAddress,
		httpx.Auth(d.Tokens, publicRoute, d.Log),
		httpx.NewRateLimiter(d.RateLimitPerMinute).ByUser,
	)
}

// GetHealthz checks the database within a second.
func (s *Server) GetHealthz(ctx context.Context, _ api.GetHealthzRequestObject) (api.GetHealthzResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		return api.GetHealthz503JSONResponse{ErrorJSONResponse: api.ErrorJSONResponse{Error: errorBody("db_unavailable", "database unreachable")}}, nil
	}
	return api.GetHealthz200JSONResponse{Status: api.Ok}, nil
}

func errorBody(code, message string) api.ErrorBody {
	return api.ErrorBody{Code: code, Message: message}
}
