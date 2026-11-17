package server

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/habos/linguascroll/backend/internal/api"
	"github.com/habos/linguascroll/backend/internal/engagement"
	"github.com/habos/linguascroll/backend/internal/platform/auth"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
)

// Module V4 engagement endpoints (internal/engagement).

func currentUser(ctx context.Context) (uuid.UUID, error) {
	u, ok := auth.UserFrom(ctx)
	if !ok {
		return uuid.Nil, httpx.E(http.StatusUnauthorized, "unauthorized", "missing or invalid token")
	}
	return u.ID, nil
}

// do runs f for the current user.
func (s *Server) do(ctx context.Context, f func(uuid.UUID) error) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	return f(u)
}

func (s *Server) LikeVideo(ctx context.Context, r api.LikeVideoRequestObject) (api.LikeVideoResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Like(ctx, u, r.Id) }); err != nil {
		return nil, err
	}
	return api.LikeVideo204Response{}, nil
}

func (s *Server) UnlikeVideo(ctx context.Context, r api.UnlikeVideoRequestObject) (api.UnlikeVideoResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Unlike(ctx, u, r.Id) }); err != nil {
		return nil, err
	}
	return api.UnlikeVideo204Response{}, nil
}

func (s *Server) SaveVideo(ctx context.Context, r api.SaveVideoRequestObject) (api.SaveVideoResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Save(ctx, u, r.Id) }); err != nil {
		return nil, err
	}
	return api.SaveVideo204Response{}, nil
}

func (s *Server) UnsaveVideo(ctx context.Context, r api.UnsaveVideoRequestObject) (api.UnsaveVideoResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Unsave(ctx, u, r.Id) }); err != nil {
		return nil, err
	}
	return api.UnsaveVideo204Response{}, nil
}

func (s *Server) HideVideo(ctx context.Context, r api.HideVideoRequestObject) (api.HideVideoResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Hide(ctx, u, r.Id) }); err != nil {
		return nil, err
	}
	return api.HideVideo204Response{}, nil
}

func (s *Server) UnhideVideo(ctx context.Context, r api.UnhideVideoRequestObject) (api.UnhideVideoResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Unhide(ctx, u, r.Id) }); err != nil {
		return nil, err
	}
	return api.UnhideVideo204Response{}, nil
}

func (s *Server) BlockChannel(ctx context.Context, r api.BlockChannelRequestObject) (api.BlockChannelResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.BlockChannel(ctx, u, r.ChannelId) }); err != nil {
		return nil, err
	}
	return api.BlockChannel204Response{}, nil
}

func (s *Server) UnblockChannel(ctx context.Context, r api.UnblockChannelRequestObject) (api.UnblockChannelResponseObject, error) {
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.UnblockChannel(ctx, u, r.ChannelId) }); err != nil {
		return nil, err
	}
	return api.UnblockChannel204Response{}, nil
}

func (s *Server) ListChannelBlocks(ctx context.Context, _ api.ListChannelBlocksRequestObject) (api.ListChannelBlocksResponseObject, error) {
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	bs, err := s.engagement.ChannelBlocks(ctx, u)
	if err != nil {
		return nil, err
	}
	out := api.ListChannelBlocks200JSONResponse{Items: make([]api.ChannelBlock, len(bs))}
	for i, b := range bs {
		out.Items[i] = api.ChannelBlock{ChannelId: b.ChannelID, ChannelTitle: b.Title, BlockedAt: b.BlockedAt}
	}
	return out, nil
}

func (s *Server) ReportVideo(ctx context.Context, r api.ReportVideoRequestObject) (api.ReportVideoResponseObject, error) {
	if r.Body == nil {
		return nil, httpx.E(http.StatusBadRequest, "bad_request", "body required")
	}
	if err := s.do(ctx, func(u uuid.UUID) error { return s.engagement.Report(ctx, u, r.Id, string(r.Body.Reason)) }); err != nil {
		return nil, err
	}
	return api.ReportVideo204Response{}, nil
}

func (s *Server) PostEvents(ctx context.Context, r api.PostEventsRequestObject) (api.PostEventsResponseObject, error) {
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if r.Body == nil {
		return nil, httpx.E(http.StatusBadRequest, "bad_request", "body required")
	}
	evs := make([]engagement.Event, len(r.Body.Events))
	for i, e := range r.Body.Events {
		evs[i] = engagement.Event{ID: e.Id, Type: string(e.Type), At: e.At, VideoID: e.VideoId}
		if e.Props != nil {
			evs[i].Props = *e.Props
		}
	}
	res, err := s.engagement.Ingest(ctx, u, evs)
	if err != nil {
		return nil, err
	}
	return api.PostEvents202JSONResponse{Accepted: res.Accepted, Duplicates: res.Duplicates, Dropped: res.Dropped}, nil
}

func (s *Server) ListMyLikes(ctx context.Context, r api.ListMyLikesRequestObject) (api.ListMyLikesResponseObject, error) {
	page, err := s.cardPage(ctx, r.Params.Cursor, r.Params.Limit, s.engagement.Likes)
	if err != nil {
		return nil, err
	}
	return api.ListMyLikes200JSONResponse(page), nil
}

func (s *Server) ListMySaves(ctx context.Context, r api.ListMySavesRequestObject) (api.ListMySavesResponseObject, error) {
	page, err := s.cardPage(ctx, r.Params.Cursor, r.Params.Limit, s.engagement.Saves)
	if err != nil {
		return nil, err
	}
	return api.ListMySaves200JSONResponse(page), nil
}

type lister func(context.Context, uuid.UUID, *engagement.ListCursor, int) ([]engagement.Card, *engagement.ListCursor, error)

func (s *Server) cardPage(ctx context.Context, cursor *string, limit *int, list lister) (api.VideoCardPage, error) {
	u, err := currentUser(ctx)
	if err != nil {
		return api.VideoCardPage{}, err
	}
	var raw string
	if cursor != nil {
		raw = *cursor
	}
	cur, err := engagement.DecodeListCursor(raw)
	if err != nil {
		return api.VideoCardPage{}, httpx.E(http.StatusBadRequest, "bad_cursor", "cursor is not valid")
	}
	n := 30
	if limit != nil {
		n = *limit
	}
	if n < 1 || n > 100 {
		return api.VideoCardPage{}, httpx.E(http.StatusBadRequest, "bad_request", "limit must be 1 to 100")
	}
	cards, next, err := list(ctx, u, cur, n)
	if err != nil {
		return api.VideoCardPage{}, err
	}
	page := api.VideoCardPage{Items: make([]api.VideoCard, len(cards))}
	for i, c := range cards {
		var channel string
		if c.ChannelID != nil {
			channel = *c.ChannelID
		}
		page.Items[i] = api.VideoCard{
			Id: c.ID, Source: api.Youtube, Language: c.Language, Level: c.Level, DurationMs: c.DurationMS, Aspect: float32(c.Aspect),
			Youtube: api.YouTubeInfo{VideoId: c.YouTubeID, ChannelId: channel, ChannelTitle: c.ChannelTitle, Title: c.Title, HasCaptions: c.HasCaptions},
			Viewer:  api.ViewerState{Liked: c.Liked, Saved: c.Saved},
		}
	}
	if next != nil {
		enc := next.Encode()
		page.NextCursor = &enc
	}
	return page, nil
}
