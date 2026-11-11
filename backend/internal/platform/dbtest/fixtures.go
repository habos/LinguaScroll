package dbtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Clip overrides for InsertClips; zero values get defaults.
type Clip struct {
	Level        *int
	CategoryID   string // default "27" (Education); "10" is Music
	ViewCount    int64
	LikeCount    int64
	PublishedAgo time.Duration
}

// InsertClips adds n live YouTube clips in language, spread round-robin
// across channels channels (channel IDs "UCtest_<lang>_<k>"), and returns
// their IDs in insert order. tweak, if set, can change each clip.
func InsertClips(t testing.TB, pool *pgxpool.Pool, language string, n, channels int, tweak func(i int, c *Clip)) []uuid.UUID {
	t.Helper()
	if channels < 1 {
		channels = 1
	}
	ctx := context.Background()
	ids := make([]uuid.UUID, 0, n)
	for i := range n {
		c := Clip{CategoryID: "27", ViewCount: int64(1000 * (i + 1)), LikeCount: int64(20 * (i + 1)), PublishedAgo: time.Duration(i) * 24 * time.Hour}
		if tweak != nil {
			tweak(i, &c)
		}
		id := uuid.Must(uuid.NewV7())
		ch := fmt.Sprintf("UCtest_%s_%d", language, i%channels)
		levelSource := any(nil)
		if c.Level != nil {
			levelSource = "manual"
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO videos (id, source, youtube_video_id, youtube_channel_id, youtube_channel_title, youtube_title,
			                    youtube_category_id, language, language_source, language_confidence, level, level_source,
			                    duration_ms, aspect, view_count, like_count, has_captions, published_at)
			VALUES ($1, 'youtube', $2, $3, $3, $4, $5, $6, 'manual', 1, $7, $8, 30000, 0.5625, $9, $10, true, now() - $11::interval)`,
			id, fmt.Sprintf("yt_%s_%d_%s", language, i, id.String()[:8]), ch, fmt.Sprintf("Test clip %d", i),
			c.CategoryID, language, c.Level, levelSource, c.ViewCount, c.LikeCount, c.PublishedAgo.String())
		if err != nil {
			t.Fatalf("insert clip %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	return ids
}
