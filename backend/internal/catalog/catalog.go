// Package catalog is module V3: it builds and maintains each language's
// catalog of YouTube Shorts from YouTube Data API metadata only. It finds
// clips, decides their language and rough level, keeps their stats fresh,
// and stays within quota and YouTube's 30-day storage rule.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/habos/linguascroll/backend/internal/appconfig"
	"github.com/habos/linguascroll/backend/internal/langdetect"
	"github.com/habos/linguascroll/backend/internal/youtube"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tunables from the V3 rules.
const (
	MaxDurationMs     = 180_000
	RefreshAfter      = 25 * 24 * time.Hour // re-read before YouTube's 30-day limit
	EngagementMinDone = 20                  // completed views before engagement sets the level
	descriptionChars  = 500
)

// ErrQuotaSpent means today's unit budget is used up (UTC day).
var ErrQuotaSpent = errors.New("catalog: daily YouTube quota spent")

// Service runs the catalog jobs.
type Service struct {
	DB     *pgxpool.Pool
	YT     youtube.Client
	Detect langdetect.Detector
	Log    *slog.Logger
	Now    func() time.Time // nil means time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *Service) cfg() appconfig.Store { return appconfig.Store{DB: s.DB} }

// reserve books units against today's budget, or returns ErrQuotaSpent.
func (s *Service) reserve(ctx context.Context, units int) error {
	budget, err := appconfig.Get(ctx, s.cfg(), "youtube.daily_quota_units", 9000)
	if err != nil {
		return err
	}
	var total int
	err = s.DB.QueryRow(ctx, `
		INSERT INTO youtube_quota AS q (day, units) SELECT $1::date, $2::int WHERE $2::int <= $3::int
		ON CONFLICT (day) DO UPDATE SET units = q.units + EXCLUDED.units
		 WHERE q.units + EXCLUDED.units <= $3::int
		RETURNING units`, s.now().Format("2006-01-02"), units, budget).Scan(&total)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrQuotaSpent
	}
	return err
}

// UnitsUsedToday reports today's spent units.
func (s *Service) UnitsUsedToday(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT coalesce((SELECT units FROM youtube_quota WHERE day = $1), 0)`, s.now().Format("2006-01-02")).Scan(&n)
	return n, err
}

type source struct {
	ID        uuid.UUID
	Language  string
	Kind      string // query | channel
	Value     string
	LevelHint *int
	RunCount  int
}

// windows rotate publishedAfter/Before so repeat searches return new clips:
// [0,30d), [30d,90d), [90d,180d), [180d,1y), [1y,2y), [2y, ∞).
var windows = [][2]time.Duration{
	{0, 30 * 24 * time.Hour}, {30 * 24 * time.Hour, 90 * 24 * time.Hour}, {90 * 24 * time.Hour, 180 * 24 * time.Hour},
	{180 * 24 * time.Hour, 365 * 24 * time.Hour}, {365 * 24 * time.Hour, 730 * 24 * time.Hour}, {730 * 24 * time.Hour, 0},
}

// RunSearches runs up to max searches (each 100 units plus 1 per 50 new
// IDs), stopping early when the daily budget is spent. It returns how many
// searches ran and how many clips were kept.
func (s *Service) RunSearches(ctx context.Context, maxSearches int) (searches, kept int, err error) {
	for searches < maxSearches {
		k, err := s.SearchOnce(ctx)
		if errors.Is(err, ErrQuotaSpent) || errors.Is(err, youtube.ErrQuotaExceeded) {
			return searches, kept, nil
		}
		if errors.Is(err, errNoSources) {
			return searches, kept, nil
		}
		if err != nil {
			return searches, kept, err
		}
		searches++
		kept += k
	}
	return searches, kept, nil
}

var errNoSources = errors.New("catalog: no enabled sources")

// pickSource chooses the language that has had the fewest searches today,
// weighted towards languages with fewer live clips, then that language's
// least recently run source.
func (s *Service) pickSource(ctx context.Context) (source, error) {
	var src source
	err := s.DB.QueryRow(ctx, `
		WITH langs AS (
			SELECT l.code, l.launch_order,
			       (SELECT count(*) FROM youtube_search_log g WHERE g.language = l.code AND g.ran_at >= $1) AS searched,
			       (SELECT count(*) FROM videos v WHERE v.language = l.code AND v.status = 'live') AS live
			  FROM languages l
			 WHERE l.is_learnable AND EXISTS (SELECT 1 FROM youtube_sources ys WHERE ys.language = l.code AND ys.enabled)
		), lang AS (
			SELECT code FROM langs ORDER BY searched + live / 500.0, launch_order NULLS LAST, code LIMIT 1
		)
		SELECT ys.id, ys.language, ys.kind, ys.value, ys.level_hint, ys.run_count
		  FROM youtube_sources ys JOIN lang ON lang.code = ys.language
		 WHERE ys.enabled
		 ORDER BY ys.last_run_at NULLS FIRST, ys.value
		 LIMIT 1`, startOfDay(s.now())).Scan(&src.ID, &src.Language, &src.Kind, &src.Value, &src.LevelHint, &src.RunCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return source{}, errNoSources
	}
	return src, err
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// SearchOnce runs one search for the source pickSource chooses and stores
// the clips that pass the filters. It returns the number kept.
func (s *Service) SearchOnce(ctx context.Context) (int, error) {
	src, err := s.pickSource(ctx)
	if err != nil {
		return 0, err
	}
	if err := s.reserve(ctx, youtube.SearchCost); err != nil {
		return 0, err
	}
	units := youtube.SearchCost
	w := windows[src.RunCount%len(windows)]
	q := youtube.SearchQuery{RelevanceLanguage: primarySubtag(src.Language)}
	if w[1] != 0 {
		q.PublishedAfter = s.now().Add(-w[1])
	}
	if w[0] != 0 {
		q.PublishedBefore = s.now().Add(-w[0])
	}
	if src.Kind == "channel" {
		q.ChannelID = src.Value
	} else {
		q.Query = src.Value
	}
	ids, _, err := s.YT.Search(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("search %s %q: %w", src.Language, src.Value, err)
	}
	fresh, err := s.unknownIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	var vids []youtube.Video
	for start := 0; start < len(fresh); start += youtube.MaxIDs {
		batch := fresh[start:min(start+youtube.MaxIDs, len(fresh))]
		if err := s.reserve(ctx, youtube.VideosCost); err != nil {
			break // keep what we have; the rest is found again later
		}
		units += youtube.VideosCost
		vs, err := s.YT.Videos(ctx, batch)
		if err != nil {
			return 0, fmt.Errorf("videos: %w", err)
		}
		vids = append(vids, vs...)
	}
	kept, err := s.store(ctx, src, vids)
	if err != nil {
		return 0, err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE youtube_sources SET last_run_at = now(), run_count = run_count + 1 WHERE id = $1`, src.ID); err != nil {
		return 0, err
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO youtube_search_log (id, source_id, language, ran_at, units, found, kept) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.Must(uuid.NewV7()), src.ID, src.Language, s.now(), units, len(ids), kept); err != nil {
		return 0, err
	}
	s.log().Info("youtube search", "language", src.Language, "source", src.Value, "found", len(ids), "new", len(fresh), "kept", kept, "units", units)
	return kept, nil
}

func (s *Service) unknownIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, `
		SELECT youtube_video_id FROM videos WHERE youtube_video_id = ANY($1)
		UNION ALL
		SELECT youtube_video_id FROM youtube_rejects WHERE youtube_video_id = ANY($1) AND rejected_at > $2`,
		ids, s.now().Add(-RejectTTL))
	if err != nil {
		return nil, err
	}
	known, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, k := range known {
		seen[k] = true
	}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// Keep reports whether a clip passes the v0 filters: 180 s or shorter,
// embeddable, public, not made for kids, not age-restricted, vertical.
func Keep(v youtube.Video) bool {
	return v.DurationMs > 0 && v.DurationMs <= MaxDurationMs && v.Embeddable && v.Public &&
		!v.MadeForKids && !v.AgeRestricted && v.EmbedHeight > v.EmbedWidth
}

// Classification is how a clip's language was decided.
type Classification struct {
	Language   string
	Source     string // audio_tag | seed_channel | text_detect
	Confidence float64
}

// ClassifyLanguage applies the V3 rules for target language; the first rule
// that applies wins. ok=false means drop the clip.
func ClassifyLanguage(v youtube.Video, target string, seedChannels map[string]bool, det langdetect.Detector, minConfidence float64) (Classification, bool) {
	if v.DefaultAudioLanguage != "" {
		if strings.EqualFold(primarySubtag(v.DefaultAudioLanguage), primarySubtag(target)) {
			return Classification{target, "audio_tag", 1}, true
		}
		return Classification{}, false
	}
	if seedChannels[v.ChannelID] {
		return Classification{target, "seed_channel", 1}, true
	}
	if det != nil {
		lang, conf := det.Detect(detectText(v))
		if lang == target && conf >= minConfidence {
			return Classification{target, "text_detect", conf}, true
		}
	}
	return Classification{}, false
}

func detectText(v youtube.Video) string {
	desc := v.Description
	if r := []rune(desc); len(r) > descriptionChars {
		desc = string(r[:descriptionChars])
	}
	return strings.Join(append([]string{v.Title, desc}, v.Tags...), "\n")
}

func primarySubtag(code string) string {
	if i := strings.IndexAny(code, "-_"); i > 0 {
		return code[:i]
	}
	return code
}

// LevelKeywords maps a level (1 to 6) to words that suggest it.
type LevelKeywords map[int][]string

// GuessLevel applies the V3 level rules: the source's hint, else the lowest
// level whose keyword appears in the title, description or tags, else nil.
func GuessLevel(v youtube.Video, hint *int, kw LevelKeywords) (level *int, source string) {
	if hint != nil {
		return hint, "hint"
	}
	text := strings.ToLower(detectText(v))
	levels := make([]int, 0, len(kw))
	for l := range kw {
		levels = append(levels, l)
	}
	sort.Ints(levels)
	for _, l := range levels {
		for _, w := range kw[l] {
			if containsWord(text, strings.ToLower(w)) {
				l := l
				return &l, "keyword"
			}
		}
	}
	return nil, ""
}

// containsWord matches whole words for Latin-script keywords ("a1" must not
// match "a10") and plain substrings for scripts without spaces.
func containsWord(text, w string) bool {
	if w == "" {
		return false
	}
	ascii := true
	for _, r := range w {
		if r > unicode.MaxLatin1 && !unicode.Is(unicode.Latin, r) {
			ascii = false
			break
		}
	}
	if !ascii {
		return strings.Contains(text, w)
	}
	re, err := regexp.Compile(`(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(w) + `($|[^\p{L}\p{N}])`)
	return err == nil && re.MatchString(text)
}

func (s *Service) levelKeywords(ctx context.Context) (LevelKeywords, error) {
	raw, err := appconfig.Get(ctx, s.cfg(), "youtube.level_keywords", map[string][]string{})
	if err != nil {
		return nil, err
	}
	kw := LevelKeywords{}
	for k, words := range raw {
		l, err := strconv.Atoi(k)
		if err != nil || l < 1 || l > 6 {
			return nil, fmt.Errorf("youtube.level_keywords: bad level %q", k)
		}
		kw[l] = words
	}
	return kw, nil
}

func (s *Service) seedChannels(ctx context.Context, lang string) (map[string]bool, error) {
	rows, err := s.DB.Query(ctx, `SELECT value FROM youtube_sources WHERE language = $1 AND kind = 'channel' AND enabled`, lang)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m, err
}

func (s *Service) store(ctx context.Context, src source, vids []youtube.Video) (int, error) {
	if len(vids) == 0 {
		return 0, nil
	}
	minConf, err := appconfig.Get(ctx, s.cfg(), "youtube.min_language_confidence", 0.8)
	if err != nil {
		return 0, err
	}
	kw, err := s.levelKeywords(ctx)
	if err != nil {
		return 0, err
	}
	seeds, err := s.seedChannels(ctx, src.Language)
	if err != nil {
		return 0, err
	}
	kept := 0
	for _, v := range vids {
		if !Keep(v) {
			if err := s.reject(ctx, v.ID, "filters"); err != nil {
				return kept, err
			}
			continue
		}
		c, ok := ClassifyLanguage(v, src.Language, seeds, s.Detect, minConf)
		if !ok {
			if err := s.reject(ctx, v.ID, "language"); err != nil {
				return kept, err
			}
			continue
		}
		level, levelSource := GuessLevel(v, src.LevelHint, kw)
		var ls any
		if levelSource != "" {
			ls = levelSource
		}
		tag, err := s.DB.Exec(ctx, `
			INSERT INTO videos (id, source, youtube_video_id, youtube_channel_id, youtube_channel_title, youtube_title,
			                    youtube_description, youtube_category_id, youtube_tags, language, language_source,
			                    language_confidence, level, level_source, duration_ms, aspect, view_count, like_count,
			                    has_captions, status, published_at, refreshed_at)
			VALUES ($1, 'youtube', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, 'live', $19, now())
			ON CONFLICT (youtube_video_id) DO NOTHING`,
			uuid.Must(uuid.NewV7()), v.ID, v.ChannelID, v.ChannelTitle, v.Title, v.Description, v.CategoryID,
			nonNil(v.Tags), c.Language, c.Source, c.Confidence, level, ls, v.DurationMs,
			float64(v.EmbedWidth)/float64(v.EmbedHeight), v.ViewCount, v.LikeCount, v.HasCaptions, v.PublishedAt)
		if err != nil {
			return kept, fmt.Errorf("store %s: %w", v.ID, err)
		}
		kept += int(tag.RowsAffected())
	}
	return kept, nil
}

// RejectTTL is how long a rejected ID is skipped before it may be checked
// again (a clip can become embeddable or public later).
const RejectTTL = 30 * 24 * time.Hour

func (s *Service) reject(ctx context.Context, id, reason string) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO youtube_rejects (youtube_video_id, reason, rejected_at) VALUES ($1, $2, $3)
		ON CONFLICT (youtube_video_id) DO UPDATE SET reason = EXCLUDED.reason, rejected_at = EXCLUDED.rejected_at`,
		id, reason, s.now())
	return err
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Refresh re-reads every live clip last refreshed more than RefreshAfter
// ago, 50 per videos.list call. Clips that are gone, private or no longer
// embeddable become removed with their text cleared. It returns how many
// clips were refreshed and removed.
func (s *Service) Refresh(ctx context.Context) (refreshed, removed int, err error) {
	if _, err := s.DB.Exec(ctx, `DELETE FROM youtube_rejects WHERE rejected_at < $1`, s.now().Add(-RejectTTL)); err != nil {
		return 0, 0, err
	}
	for {
		rows, err := s.DB.Query(ctx, `
			SELECT youtube_video_id FROM videos
			 WHERE source = 'youtube' AND status = 'live' AND refreshed_at < $1
			 ORDER BY refreshed_at LIMIT $2`, s.now().Add(-RefreshAfter), youtube.MaxIDs)
		if err != nil {
			return refreshed, removed, err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil || len(ids) == 0 {
			return refreshed, removed, err
		}
		if err := s.reserve(ctx, youtube.VideosCost); err != nil {
			if errors.Is(err, ErrQuotaSpent) {
				return refreshed, removed, nil // tomorrow
			}
			return refreshed, removed, err
		}
		vs, err := s.YT.Videos(ctx, ids)
		if err != nil {
			return refreshed, removed, err
		}
		got := map[string]youtube.Video{}
		for _, v := range vs {
			got[v.ID] = v
		}
		for _, id := range ids {
			v, ok := got[id]
			if !ok || !v.Embeddable || !v.Public {
				if _, err := s.DB.Exec(ctx, `
					UPDATE videos SET status = 'removed', hold_reason = 'youtube_unavailable', refreshed_at = now(),
					       youtube_title = NULL, youtube_description = NULL, youtube_tags = '{}', youtube_channel_title = NULL
					 WHERE youtube_video_id = $1`, id); err != nil {
					return refreshed, removed, err
				}
				removed++
				continue
			}
			if _, err := s.DB.Exec(ctx, `
				UPDATE videos SET youtube_title = $2, youtube_description = $3, youtube_tags = $4, youtube_channel_title = $5,
				       youtube_category_id = $6, view_count = $7, like_count = $8, has_captions = $9, refreshed_at = now()
				 WHERE youtube_video_id = $1`,
				id, v.Title, v.Description, nonNil(v.Tags), v.ChannelTitle, v.CategoryID, v.ViewCount, v.LikeCount, v.HasCaptions); err != nil {
				return refreshed, removed, err
			}
			refreshed++
		}
	}
}

// RefineLevels sets level from engagement once a clip has
// EngagementMinDone completed views: the rounded average level of those
// viewers. It overrides keyword guesses. Returns the clips updated.
func (s *Service) RefineLevels(ctx context.Context) (int, error) {
	tag, err := s.DB.Exec(ctx, `
		UPDATE videos v
		   SET level = s.level, level_source = 'engagement'
		  FROM (SELECT video_id, round(sum(viewer_level * completions)::numeric / sum(completions))::int AS level
		          FROM video_level_stats GROUP BY video_id HAVING sum(completions) >= $1) s
		 WHERE v.id = s.video_id AND (v.level IS DISTINCT FROM s.level OR v.level_source IS DISTINCT FROM 'engagement')`,
		EngagementMinDone)
	return int(tag.RowsAffected()), err
}
