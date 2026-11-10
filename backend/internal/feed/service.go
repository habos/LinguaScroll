package feed

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUnknownLanguage means the language isn't a learnable language code.
var ErrUnknownLanguage = errors.New("unknown language")

// Seen windows: a clip served to a device is held back for 30 days, or 7
// when fewer than a page of fresh clips remain.
const (
	seenWindow         = 30 * 24 * time.Hour
	seenWindowFallback = 7 * 24 * time.Hour
)

// Service serves feed pages.
type Service struct {
	DB *pgxpool.Pool
	// Now and Rand are overridable for tests.
	Now  func() time.Time
	Rand func() *rand.Rand
}

// Item is one served clip and the device's state for it.
type Item struct {
	Clip
	Liked, Saved bool
}

// Page is one feed page. Next is nil when nothing is left to serve.
type Page struct {
	Items []Item
	Next  *Cursor
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) rng() *rand.Rand {
	if s.Rand != nil {
		return s.Rand()
	}
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}

// Next returns the next page for user in language after cur, and records
// the served clips in feed_seen.
func (s Service) Next(ctx context.Context, user uuid.UUID, language string, cur Cursor) (Page, error) {
	cfg, err := LoadConfig(ctx, s.DB)
	if err != nil {
		return Page{}, fmt.Errorf("feed config: %w", err)
	}
	var learnable bool
	err = s.DB.QueryRow(ctx, `SELECT is_learnable FROM languages WHERE code = $1`, language).Scan(&learnable)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !learnable) {
		return Page{}, ErrUnknownLanguage
	}
	if err != nil {
		return Page{}, err
	}

	prof, err := s.profile(ctx, user, language)
	if err != nil {
		return Page{}, fmt.Errorf("feed profile: %w", err)
	}
	now := s.now()
	rng := s.rng()
	exploreN := ExploreSlots(cfg.PageSize, cfg.ExploreShare, rng)

	ranked, explore, err := s.candidates(ctx, user, language, prof.Level, cfg, exploreN, seenWindow)
	if err != nil {
		return Page{}, err
	}
	if len(ranked) < cfg.PageSize {
		if ranked, explore, err = s.candidates(ctx, user, language, prof.Level, cfg, exploreN, seenWindowFallback); err != nil {
			return Page{}, err
		}
	}
	Rank(ranked, prof, cfg.Weights, now, rng)
	clips := Assemble(ranked, explore, cfg.PageSize, exploreN, cfg.ChannelSpacing, cfg.MusicCap, cur.Recent, rng)
	if len(clips) == 0 {
		return Page{Items: []Item{}}, nil
	}

	items, err := s.viewerState(ctx, user, clips)
	if err != nil {
		return Page{}, err
	}
	if err := s.markSeen(ctx, user, clips); err != nil {
		return Page{}, err
	}
	next := Cursor{Page: cur.Page + 1, Recent: RecentChannels(cur.Recent, clips, cfg.ChannelSpacing-1)}
	return Page{Items: items, Next: &next}, nil
}

// profile loads the device's level in language and its recent behaviour.
func (s Service) profile(ctx context.Context, user uuid.UUID, language string) (Profile, error) {
	p := Profile{ChannelAffinity: map[string]float64{}, CategoryShare: map[string]float64{}}
	err := s.DB.QueryRow(ctx, `SELECT level FROM learner_languages WHERE user_id = $1 AND language = $2`, user, language).Scan(&p.Level)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}

	// k = 0.5: completed >= 2 clips of the channel in the last 30 days,
	// read from the last 100 views, which also give g.
	rows, err := s.DB.Query(ctx, `
		SELECT coalesce(v.youtube_channel_id, ''), coalesce(v.youtube_category_id, ''),
		       e.props->'completed' = 'true'::jsonb AS completed,
		       e.at > now() - interval '30 days' AS recent
		  FROM events e
		  JOIN videos v ON v.id = e.video_id
		 WHERE e.user_id = $1 AND e.type = 'view_end'
		   AND e.at > now() - interval '180 days'
		   AND jsonb_typeof(e.props->'watched_ms') = 'number'
		   AND (e.props->>'watched_ms')::numeric >= 2000
		 ORDER BY e.at DESC
		 LIMIT 100`, user)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	total := 0
	completedByCat := map[string]int{}
	completedByChan := map[string]int{}
	for rows.Next() {
		var ch, cat string
		var completed, recent *bool
		if err := rows.Scan(&ch, &cat, &completed, &recent); err != nil {
			return p, err
		}
		total++
		if completed != nil && *completed {
			completedByCat[cat]++
			if recent != nil && *recent {
				completedByChan[ch]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	for cat, n := range completedByCat {
		p.CategoryShare[cat] = float64(n) / float64(total)
	}
	for ch, n := range completedByChan {
		if n >= 2 {
			p.ChannelAffinity[ch] = 0.5
		}
	}

	// k = 1: liked or saved a clip from the channel in the last 30 days.
	rows, err = s.DB.Query(ctx, `
		SELECT DISTINCT v.youtube_channel_id
		  FROM (SELECT video_id FROM likes WHERE user_id = $1 AND created_at > now() - interval '30 days'
		        UNION
		        SELECT video_id FROM saves WHERE user_id = $1 AND created_at > now() - interval '30 days') x
		  JOIN videos v ON v.id = x.video_id
		 WHERE v.youtube_channel_id IS NOT NULL`, user)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return p, err
		}
		p.ChannelAffinity[ch] = 1
	}
	return p, rows.Err()
}

// candidatesSQL filters once and takes both pools from the same set: the
// top $7 by base score, and $9 random clips with fewer than $8 in-app views
// for explore. $1 user, $2 language, $3..$4 level range, $5..$6 seen cutoff
// and level-unknown flag are explicit values (no "$x IS NULL OR" clauses),
// so Postgres's cached generic plan stays good.
const candidatesSQL = `
	WITH c AS MATERIALIZED (
		SELECT v.id, v.youtube_video_id, coalesce(v.youtube_channel_id, '') AS channel_id,
		       coalesce(v.youtube_channel_title, '') AS channel_title, coalesce(v.youtube_title, '') AS title,
		       coalesce(v.youtube_category_id, '') AS category_id, v.level, v.duration_ms, v.aspect,
		       v.has_captions, v.published_at,
		       coalesce(s.impressions, 0) AS impressions, coalesce(s.views, 0) AS views,
		       coalesce(s.completions, 0) AS completions, coalesce(s.hides, 0) AS hides,
		       coalesce(s.views_pct, 0) AS views_pct, coalesce(s.likes_pct, 0) AS likes_pct,
		       coalesce(s.base_score, 0) AS base_score
		  FROM videos v
		  LEFT JOIN video_stats s ON s.video_id = v.id
		 WHERE v.language = $2 AND v.status = 'live' AND v.source = 'youtube'
		   AND (v.level IS NULL OR v.level BETWEEN $3 AND $4)
		   AND NOT EXISTS (SELECT 1 FROM hides h WHERE h.user_id = $1 AND h.video_id = v.id)
		   AND NOT EXISTS (SELECT 1 FROM channel_blocks b WHERE b.user_id = $1 AND b.youtube_channel_id = v.youtube_channel_id)
		   AND NOT EXISTS (SELECT 1 FROM feed_seen f WHERE f.user_id = $1 AND f.video_id = v.id AND f.served_at > $5)
	)
	(SELECT *, false AS explore FROM c ORDER BY base_score DESC, id LIMIT $6)
	UNION ALL
	(SELECT *, true AS explore FROM c WHERE views < $7 ORDER BY random() LIMIT $8)`

// candidates returns the top cfg.Candidates by base score, and a random
// explore pool of clips with few in-app views.
func (s Service) candidates(ctx context.Context, user uuid.UUID, language string, level *int, cfg Config, exploreN int, seen time.Duration) (ranked, explore []Clip, err error) {
	lo, hi := 1, 6
	if level != nil {
		lo, hi = *level-cfg.LevelWindow, *level+cfg.LevelWindow
	}
	rows, err := s.DB.Query(ctx, candidatesSQL, user, language, lo, hi, s.now().Add(-seen),
		cfg.Candidates, cfg.ExploreMaxViews, exploreN*5)
	if err != nil {
		return nil, nil, fmt.Errorf("feed candidates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Clip
		var viewsPct, likesPct, base float32
		var isExplore bool
		if err := rows.Scan(&c.ID, &c.YouTubeID, &c.ChannelID, &c.ChannelTitle, &c.Title, &c.CategoryID, &c.Level,
			&c.DurationMS, &c.Aspect, &c.HasCaptions, &c.PublishedAt,
			&c.Impressions, &c.Views, &c.Completions, &c.Hides, &viewsPct, &likesPct, &base, &isExplore); err != nil {
			return nil, nil, err
		}
		c.ViewsPct, c.LikesPct = float64(viewsPct), float64(likesPct)
		if isExplore {
			explore = append(explore, c)
		} else {
			ranked = append(ranked, c)
		}
	}
	return ranked, explore, rows.Err()
}

func (s Service) viewerState(ctx context.Context, user uuid.UUID, clips []Clip) ([]Item, error) {
	ids := make([]uuid.UUID, len(clips))
	for i, c := range clips {
		ids[i] = c.ID
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id,
		       EXISTS (SELECT 1 FROM likes l WHERE l.user_id = $1 AND l.video_id = id),
		       EXISTS (SELECT 1 FROM saves s WHERE s.user_id = $1 AND s.video_id = id)
		  FROM unnest($2::uuid[]) AS id`, user, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	state := map[uuid.UUID][2]bool{}
	for rows.Next() {
		var id uuid.UUID
		var liked, saved bool
		if err := rows.Scan(&id, &liked, &saved); err != nil {
			return nil, err
		}
		state[id] = [2]bool{liked, saved}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]Item, len(clips))
	for i, c := range clips {
		items[i] = Item{Clip: c, Liked: state[c.ID][0], Saved: state[c.ID][1]}
	}
	return items, nil
}

func (s Service) markSeen(ctx context.Context, user uuid.UUID, clips []Clip) error {
	ids := make([]uuid.UUID, len(clips))
	for i, c := range clips {
		ids[i] = c.ID
	}
	_, err := s.DB.Exec(ctx, `
		INSERT INTO feed_seen (user_id, video_id, served_at)
		SELECT $1, id, now() FROM unnest($2::uuid[]) AS id
		ON CONFLICT (user_id, video_id) DO UPDATE SET served_at = excluded.served_at`, user, ids)
	return err
}
