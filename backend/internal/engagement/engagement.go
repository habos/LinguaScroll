// Package engagement is module V4 outside the ranking: likes, saves, hides,
// channel blocks, reports, client events and the stats rollup.
package engagement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/habos/linguascroll/backend/internal/appconfig"
	"github.com/habos/linguascroll/backend/internal/catalog"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
)

// Limits on client events.
const (
	MaxEventsPerBatch = 200
	MaxEventAge       = 7 * 24 * time.Hour
	MaxEventAhead     = time.Hour
	// ViewMinMs is how long a clip must play to count as a view.
	ViewMinMs = 2000
)

// Service holds the database and a clock (tests replace it).
type Service struct {
	DB  *pgxpool.Pool
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var errVideoNotFound = httpx.E(http.StatusNotFound, "not_found", "no live clip with this id")

// toggle is one of the user/video state tables.
type toggle struct{ table, on, off string }

var (
	likesT = toggle{"likes", "like", "unlike"}
	savesT = toggle{"saves", "save", "unsave"}
	hidesT = toggle{"hides", "hide", "unhide"}
)

// set adds (on=true) or removes the row. Adding needs a live clip;
// removing is idempotent for any ID. An event is written only when the
// state changed, so the rollup counts each change once.
func (s *Service) set(ctx context.Context, t toggle, user, video uuid.UUID, on bool) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		var q, typ string
		if on {
			var live bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM videos WHERE id = $1 AND status = 'live')`, video).Scan(&live); err != nil {
				return err
			}
			if !live {
				return errVideoNotFound
			}
			q, typ = `INSERT INTO `+t.table+` (user_id, video_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, t.on
		} else {
			q, typ = `DELETE FROM `+t.table+` WHERE user_id = $1 AND video_id = $2`, t.off
		}
		tag, err := tx.Exec(ctx, q, user, video)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return s.logEvent(ctx, tx, user, &video, typ, nil)
	})
}

// Like, Unlike, Save, Unsave, Hide and Unhide are idempotent.
func (s *Service) Like(ctx context.Context, user, video uuid.UUID) error {
	return s.set(ctx, likesT, user, video, true)
}

func (s *Service) Unlike(ctx context.Context, user, video uuid.UUID) error {
	return s.set(ctx, likesT, user, video, false)
}

func (s *Service) Save(ctx context.Context, user, video uuid.UUID) error {
	return s.set(ctx, savesT, user, video, true)
}

func (s *Service) Unsave(ctx context.Context, user, video uuid.UUID) error {
	return s.set(ctx, savesT, user, video, false)
}

func (s *Service) Hide(ctx context.Context, user, video uuid.UUID) error {
	return s.set(ctx, hidesT, user, video, true)
}

func (s *Service) Unhide(ctx context.Context, user, video uuid.UUID) error {
	return s.set(ctx, hidesT, user, video, false)
}

func validChannelID(id string) bool {
	return id != "" && len(id) <= 64 && !strings.ContainsAny(id, " /?#")
}

// BlockChannel and UnblockChannel are idempotent.
func (s *Service) BlockChannel(ctx context.Context, user uuid.UUID, channel string) error {
	if !validChannelID(channel) {
		return httpx.E(http.StatusBadRequest, "bad_channel_id", "not a YouTube channel ID")
	}
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO channel_blocks (user_id, youtube_channel_id, created_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, user, channel, s.now())
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return s.logEvent(ctx, tx, user, nil, "channel_block", map[string]any{"channel_id": channel})
	})
}

func (s *Service) UnblockChannel(ctx context.Context, user uuid.UUID, channel string) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM channel_blocks WHERE user_id = $1 AND youtube_channel_id = $2`, user, channel)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return s.logEvent(ctx, tx, user, nil, "channel_unblock", map[string]any{"channel_id": channel})
	})
}

// ChannelBlock is one blocked channel. Title is nil when no live clip of
// the channel is left (removed clips have their text cleared).
type ChannelBlock struct {
	ChannelID string
	Title     *string
	BlockedAt time.Time
}

// ChannelBlocks lists the device's blocked channels, newest first.
func (s *Service) ChannelBlocks(ctx context.Context, user uuid.UUID) ([]ChannelBlock, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT b.youtube_channel_id,
		       (SELECT v.youtube_channel_title FROM videos v
		         WHERE v.youtube_channel_id = b.youtube_channel_id AND v.status = 'live' AND v.youtube_channel_title IS NOT NULL
		         ORDER BY v.refreshed_at DESC LIMIT 1),
		       b.created_at
		  FROM channel_blocks b WHERE b.user_id = $1
		 ORDER BY b.created_at DESC, b.youtube_channel_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ChannelBlock])
}

// Report reasons.
var reportReasons = map[string]bool{"not_this_language": true, "inappropriate": true, "spam": true, "other": true}

// Report records one report per device and clip; a repeat is a no-op.
// "not_this_language" flags the clip for a language re-check. Once
// reports.hide_after distinct devices have reported a clip, it is removed
// for everyone and its YouTube data cleared.
func (s *Service) Report(ctx context.Context, user, video uuid.UUID, reason string) error {
	if !reportReasons[reason] {
		return httpx.E(http.StatusUnprocessableEntity, "bad_reason", "unknown report reason")
	}
	hideAfter, err := appconfig.Get(ctx, appconfig.Store{DB: s.DB}, "reports.hide_after", 3)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM videos WHERE id = $1)`, video).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errVideoNotFound
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO reports (id, reporter_user_id, target_id, reason, created_at) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (reporter_user_id, target_id) DO NOTHING`, uuid.Must(uuid.NewV7()), user, video, reason, s.now())
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if err := s.logEvent(ctx, tx, user, &video, "report", map[string]any{"reason": reason}); err != nil {
			return err
		}
		if reason == "not_this_language" {
			if _, err := tx.Exec(ctx, `UPDATE videos SET language_recheck = true WHERE id = $1`, video); err != nil {
				return err
			}
		}
		// Serialise concurrent reports on one clip so the threshold is
		// counted once.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM videos WHERE id = $1 FOR UPDATE`, video); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(DISTINCT reporter_user_id) FROM reports WHERE target_id = $1 AND status = 'open'`, video).Scan(&n); err != nil {
			return err
		}
		if n < hideAfter {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE videos SET `+catalog.ClearYouTubeData+`, hold_reason = 'reports' WHERE id = $1 AND status = 'live'`, video); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE reports SET status = 'actioned' WHERE target_id = $1 AND status = 'open'`, video)
		return err
	})
}

func (s *Service) logEvent(ctx context.Context, tx pgx.Tx, user uuid.UUID, video *uuid.UUID, typ string, props map[string]any) error {
	if props == nil {
		props = map[string]any{}
	}
	_, err := tx.Exec(ctx, `INSERT INTO events (id, user_id, video_id, type, at, props) VALUES ($1, $2, $3, $4, $5, $6)`,
		uuid.Must(uuid.NewV7()), user, video, typ, s.now(), props)
	return err
}

// Event is one client event.
type Event struct {
	ID      uuid.UUID
	Type    string
	At      time.Time
	VideoID *uuid.UUID
	Props   map[string]any
}

// IngestResult counts what happened to a batch.
type IngestResult struct{ Accepted, Duplicates, Dropped int }

var clientTypes = map[string]bool{"impression": true, "view_end": true, "share": true, "player_error": true}

// valid reports whether an event may be stored.
func (s *Service) valid(e Event, now time.Time) bool {
	if !clientTypes[e.Type] || e.At.Before(now.Add(-MaxEventAge)) || e.At.After(now.Add(MaxEventAhead)) {
		return false
	}
	if e.Type == "impression" || e.Type == "view_end" {
		if e.VideoID == nil {
			return false
		}
	}
	if e.Type == "view_end" {
		w, ok := e.Props["watched_ms"].(float64)
		if !ok || w < 0 {
			return false
		}
		if _, ok := e.Props["completed"].(bool); !ok {
			return false
		}
	}
	return true
}

// Ingest stores a batch of client events, deduplicated by (id, at).
func (s *Service) Ingest(ctx context.Context, user uuid.UUID, events []Event) (IngestResult, error) {
	var r IngestResult
	if len(events) == 0 {
		return r, httpx.E(http.StatusBadRequest, "bad_request", "events must not be empty")
	}
	if len(events) > MaxEventsPerBatch {
		return r, httpx.E(http.StatusBadRequest, "too_many_events", fmt.Sprintf("at most %d events per batch", MaxEventsPerBatch))
	}
	now := s.now()
	var (
		ids    []uuid.UUID
		types  []string
		ats    []time.Time
		videos []*uuid.UUID
		props  []string
	)
	for _, e := range events {
		if !s.valid(e, now) {
			r.Dropped++
			continue
		}
		p := e.Props
		if p == nil {
			p = map[string]any{}
		}
		b, err := json.Marshal(p)
		if err != nil {
			r.Dropped++
			continue
		}
		ids, types, ats, videos, props = append(ids, e.ID), append(types, e.Type), append(ats, e.At), append(videos, e.VideoID), append(props, string(b))
	}
	if len(ids) == 0 {
		return r, nil
	}
	tag, err := s.DB.Exec(ctx, `
		INSERT INTO events (id, user_id, type, at, video_id, props)
		SELECT id, $1, type, at, video_id, props::jsonb
		  FROM unnest($2::uuid[], $3::text[], $4::timestamptz[], $5::uuid[], $6::text[]) AS t(id, type, at, video_id, props)
		ON CONFLICT DO NOTHING`, user, ids, types, ats, videos, props)
	if err != nil {
		return r, err
	}
	r.Accepted = int(tag.RowsAffected())
	r.Duplicates = len(ids) - r.Accepted
	return r, nil
}

// Card is a clip in a likes or saves list.
type Card struct {
	ID                  uuid.UUID
	Language            string
	Level               *int
	DurationMS          int
	Aspect              float64
	YouTubeID           string
	ChannelID           *string
	ChannelTitle, Title string
	HasCaptions         bool
	Liked, Saved        bool
	At                  time.Time
}

// ListCursor is the keyset position in a likes or saves list.
type ListCursor struct {
	At time.Time `json:"t"`
	ID uuid.UUID `json:"v"`
}

// Encode returns the opaque form.
func (c ListCursor) Encode() string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeListCursor parses Encode's output; "" is the first page.
func DecodeListCursor(s string) (*ListCursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c ListCursor
	if err := json.Unmarshal(b, &c); err != nil || c.ID == uuid.Nil {
		return nil, errors.New("bad cursor")
	}
	return &c, nil
}

// Likes lists the device's liked live clips, newest first.
func (s *Service) Likes(ctx context.Context, user uuid.UUID, cur *ListCursor, limit int) ([]Card, *ListCursor, error) {
	return s.list(ctx, "likes", user, cur, limit)
}

// Saves lists the device's saved live clips, newest first.
func (s *Service) Saves(ctx context.Context, user uuid.UUID, cur *ListCursor, limit int) ([]Card, *ListCursor, error) {
	return s.list(ctx, "saves", user, cur, limit)
}

func (s *Service) list(ctx context.Context, table string, user uuid.UUID, cur *ListCursor, limit int) ([]Card, *ListCursor, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var at any
	var id any
	if cur != nil {
		at, id = cur.At, cur.ID
	}
	rows, err := s.DB.Query(ctx, `
		SELECT v.id, v.language, v.level, v.duration_ms, v.aspect, v.youtube_video_id, v.youtube_channel_id,
		       coalesce(v.youtube_channel_title, ''), coalesce(v.youtube_title, ''), v.has_captions,
		       EXISTS (SELECT 1 FROM likes l WHERE l.user_id = $1 AND l.video_id = v.id),
		       EXISTS (SELECT 1 FROM saves s WHERE s.user_id = $1 AND s.video_id = v.id),
		       t.created_at
		  FROM `+table+` t JOIN videos v ON v.id = t.video_id
		 WHERE t.user_id = $1 AND v.status = 'live'
		   AND ($2::timestamptz IS NULL OR (t.created_at, t.video_id) < ($2, $3::uuid))
		 ORDER BY t.created_at DESC, t.video_id DESC
		 LIMIT $4`, user, at, id, limit+1)
	if err != nil {
		return nil, nil, err
	}
	cards, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Card])
	if err != nil {
		return nil, nil, err
	}
	if len(cards) <= limit {
		return cards, nil, nil
	}
	cards = cards[:limit]
	last := cards[limit-1]
	return cards, &ListCursor{At: last.At, ID: last.ID}, nil
}
