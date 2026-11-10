// Stubs for every operation in api/openapi.yaml. When a module implements an
// endpoint, delete its stub here and add the real method to Server.

package server

import (
	"context"

	"github.com/habos/linguascroll/backend/internal/api"
)

// Unimplemented answers every operation with 501 not_implemented. Server
// embeds it, so a route only needs a real method once its module lands.
type Unimplemented struct{}

func (Unimplemented) UnblockChannel(context.Context, api.UnblockChannelRequestObject) (api.UnblockChannelResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) BlockChannel(context.Context, api.BlockChannelRequestObject) (api.BlockChannelResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) GetConfig(context.Context, api.GetConfigRequestObject) (api.GetConfigResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) PostEvents(context.Context, api.PostEventsRequestObject) (api.PostEventsResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) ListLanguages(context.Context, api.ListLanguagesRequestObject) (api.ListLanguagesResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) DeleteMe(context.Context, api.DeleteMeRequestObject) (api.DeleteMeResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) GetMe(context.Context, api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) UpdateMe(context.Context, api.UpdateMeRequestObject) (api.UpdateMeResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) DeleteMyLanguage(context.Context, api.DeleteMyLanguageRequestObject) (api.DeleteMyLanguageResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) PutMyLanguage(context.Context, api.PutMyLanguageRequestObject) (api.PutMyLanguageResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) ListMyLikes(context.Context, api.ListMyLikesRequestObject) (api.ListMyLikesResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) ListMySaves(context.Context, api.ListMySavesRequestObject) (api.ListMySavesResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) CreateAnonymousSession(context.Context, api.CreateAnonymousSessionRequestObject) (api.CreateAnonymousSessionResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) HideVideo(context.Context, api.HideVideoRequestObject) (api.HideVideoResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) UnlikeVideo(context.Context, api.UnlikeVideoRequestObject) (api.UnlikeVideoResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) LikeVideo(context.Context, api.LikeVideoRequestObject) (api.LikeVideoResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) ReportVideo(context.Context, api.ReportVideoRequestObject) (api.ReportVideoResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) UnsaveVideo(context.Context, api.UnsaveVideoRequestObject) (api.UnsaveVideoResponseObject, error) {
	return nil, ErrNotImplemented
}

func (Unimplemented) SaveVideo(context.Context, api.SaveVideoRequestObject) (api.SaveVideoResponseObject, error) {
	return nil, ErrNotImplemented
}
