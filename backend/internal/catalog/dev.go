package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/habos/linguascroll/backend/internal/youtube/fake"
)

// DevClipsPerSource is how many canned clips each enabled source finds in
// FAKE_VENDORS mode.
const DevClipsPerSource = 12

// FillDevFake makes the fake YouTube behave like a small real one for
// `make dev`: every enabled source finds DevClipsPerSource canned clips
// tagged with its language (with a level word on every third), and every
// clip already in the catalog still exists, so refresh keeps it. The IDs
// are not real videos; the player can't play them.
func FillDevFake(ctx context.Context, db *pgxpool.Pool, c *fake.Client) error {
	rows, err := db.Query(ctx, `SELECT language, kind, value FROM youtube_sources WHERE enabled`)
	if err != nil {
		return err
	}
	type src struct{ Language, Kind, Value string }
	srcs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[src])
	if err != nil {
		return err
	}
	for si, s := range srcs {
		for i := range DevClipsPerSource {
			v := fake.Video(fmt.Sprintf("dev_%s_%02d_%02d", s.Language, si, i), fmt.Sprintf("%s dev clip %d", s.Value, i+1))
			v.DefaultAudioLanguage = s.Language
			v.ChannelID = fmt.Sprintf("UCdev_%s_%d", s.Language, i%4)
			v.ChannelTitle = fmt.Sprintf("Dev channel %s %d", s.Language, i%4)
			if i%3 == 0 {
				v.Title += " para principiantes"
			}
			c.Add(s.Value, v)
		}
	}
	rows, err = db.Query(ctx, `
		SELECT youtube_video_id, coalesce(youtube_title, ''), coalesce(youtube_channel_id, ''), coalesce(youtube_category_id, ''), view_count, like_count, duration_ms
		  FROM videos WHERE source = 'youtube' AND status = 'live'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, title, channel, category string
		var views, likes int64
		var dur int
		if err := rows.Scan(&id, &title, &channel, &category, &views, &likes, &dur); err != nil {
			return err
		}
		v := fake.Video(id, title)
		v.ChannelID, v.CategoryID, v.ViewCount, v.LikeCount, v.DurationMs = channel, category, views, likes, dur
		c.Add("", v)
	}
	return rows.Err()
}
