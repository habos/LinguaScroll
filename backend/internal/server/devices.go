package server

import (
	"context"

	"github.com/habos/linguascroll/backend/internal/api"
)

// Module V2 routes, delegated to internal/devices.

func (s *Server) GetConfig(ctx context.Context, _ api.GetConfigRequestObject) (api.GetConfigResponseObject, error) {
	c, err := s.devices.Config(ctx)
	return api.GetConfig200JSONResponse(c), err
}

func (s *Server) ListLanguages(ctx context.Context, _ api.ListLanguagesRequestObject) (api.ListLanguagesResponseObject, error) {
	l, err := s.devices.Languages(ctx)
	return api.ListLanguages200JSONResponse(l), err
}

func (s *Server) CreateAnonymousSession(ctx context.Context, r api.CreateAnonymousSessionRequestObject) (api.CreateAnonymousSessionResponseObject, error) {
	res, err := s.devices.Register(ctx, *r.Body)
	if err != nil {
		return nil, err
	}
	return api.CreateAnonymousSession201JSONResponse(res), nil
}

func (s *Server) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	u, err := s.devices.Me(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse(u), nil
}

func (s *Server) UpdateMe(ctx context.Context, r api.UpdateMeRequestObject) (api.UpdateMeResponseObject, error) {
	u, err := s.devices.UpdateMe(ctx, *r.Body)
	if err != nil {
		return nil, err
	}
	return api.UpdateMe200JSONResponse(u), nil
}

func (s *Server) DeleteMe(ctx context.Context, _ api.DeleteMeRequestObject) (api.DeleteMeResponseObject, error) {
	if err := s.devices.DeleteMe(ctx); err != nil {
		return nil, err
	}
	return api.DeleteMe204Response{}, nil
}

func (s *Server) PutMyLanguage(ctx context.Context, r api.PutMyLanguageRequestObject) (api.PutMyLanguageResponseObject, error) {
	u, err := s.devices.PutLanguage(ctx, r.Code, *r.Body)
	if err != nil {
		return nil, err
	}
	return api.PutMyLanguage200JSONResponse(u), nil
}

func (s *Server) DeleteMyLanguage(ctx context.Context, r api.DeleteMyLanguageRequestObject) (api.DeleteMyLanguageResponseObject, error) {
	u, err := s.devices.DeleteLanguage(ctx, r.Code)
	if err != nil {
		return nil, err
	}
	return api.DeleteMyLanguage200JSONResponse(u), nil
}
