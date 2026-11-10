package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/habos/linguascroll/backend/internal/api"
	"github.com/habos/linguascroll/backend/internal/feed"
	"github.com/habos/linguascroll/backend/internal/platform/auth"
)

// GetFeed serves the next page of ranked clips (V4, internal/feed).
func (s *Server) GetFeed(ctx context.Context, req api.GetFeedRequestObject) (api.GetFeedResponseObject, error) {
	u, ok := auth.UserFrom(ctx)
	if !ok {
		return feedError(http.StatusUnauthorized, "unauthorized", "missing or invalid token"), nil
	}
	var raw string
	if req.Params.Cursor != nil {
		raw = *req.Params.Cursor
	}
	cur, err := feed.DecodeCursor(raw)
	if err != nil {
		return feedError(http.StatusBadRequest, "bad_cursor", "cursor is not valid"), nil
	}
	page, err := feed.Service{DB: s.db}.Next(ctx, u.ID, req.Params.Language, cur)
	if errors.Is(err, feed.ErrUnknownLanguage) {
		return feedError(http.StatusBadRequest, "unknown_language", "language is not a learnable language code"), nil
	}
	if err != nil {
		return nil, err
	}
	out := api.GetFeed200JSONResponse{Items: make([]api.VideoCard, len(page.Items))}
	for i, it := range page.Items {
		out.Items[i] = api.VideoCard{
			Id:         it.ID,
			Source:     api.Youtube,
			Language:   req.Params.Language,
			Level:      it.Level,
			DurationMs: it.DurationMS,
			Aspect:     it.Aspect,
			Youtube: api.YouTubeInfo{
				VideoId: it.YouTubeID, ChannelId: it.ChannelID, ChannelTitle: it.ChannelTitle,
				Title: it.Title, HasCaptions: it.HasCaptions,
			},
			Viewer: api.ViewerState{Liked: it.Liked, Saved: it.Saved},
		}
	}
	if page.Next != nil {
		next := page.Next.Encode()
		out.NextCursor = &next
	}
	return out, nil
}

func feedError(status int, code, message string) api.GetFeeddefaultJSONResponse {
	return api.GetFeeddefaultJSONResponse{StatusCode: status, Body: api.Error{Error: errorBody(code, message)}}
}
