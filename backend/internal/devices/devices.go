// Package devices is module V2: each install registers as an anonymous user
// (no sign-in), passes the 13+ check, and keeps its learning languages,
// levels and preferences.
package devices

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/habos/linguascroll/backend/internal/api"
	"github.com/habos/linguascroll/backend/internal/appconfig"
	"github.com/habos/linguascroll/backend/internal/platform/auth"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MinAge is the youngest allowed user.
const MinAge = 13

// Service implements the V2 endpoints.
type Service struct {
	DB  *pgxpool.Pool
	Now func() time.Time // nil means time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// AgeBand returns "teen" or "adult" for a birth year, or ok=false when the
// person is under MinAge (age = current year minus birth year).
func AgeBand(birthYear, currentYear int) (band string, ok bool) {
	age := currentYear - birthYear
	switch {
	case age < MinAge:
		return "", false
	case age < 18:
		return "teen", true
	default:
		return "adult", true
	}
}

var (
	errAgeBlocked   = httpx.E(http.StatusForbidden, "age_blocked", "LinguaScroll is for ages 13 and up")
	errLastLanguage = httpx.E(http.StatusUnprocessableEntity, "last_language", "at least one learning language must remain")
)

func unknownLanguage(code string) error {
	return httpx.E(http.StatusUnprocessableEntity, "unknown_language", "unknown or unavailable language: "+code)
}

func invalid(msg string) error {
	return httpx.E(http.StatusUnprocessableEntity, "invalid_request", msg)
}

func currentUser(ctx context.Context) (auth.CurrentUser, error) {
	u, ok := auth.UserFrom(ctx)
	if !ok {
		return auth.CurrentUser{}, httpx.E(http.StatusUnauthorized, "unauthorized", "missing bearer token")
	}
	return u, nil
}

// Config answers GET /v1/config.
func (s *Service) Config(ctx context.Context) (api.AppConfig, error) {
	cs := appconfig.Store{DB: s.DB}
	size, err := appconfig.Get(ctx, cs, "feed.page_size", 10)
	if err != nil {
		return api.AppConfig{}, err
	}
	captions, err := appconfig.Get(ctx, cs, "youtube.captions_default", true)
	if err != nil {
		return api.AppConfig{}, err
	}
	return api.AppConfig{FeedPageSize: size, YoutubeCaptionsDefault: captions}, nil
}

// Languages answers GET /v1/languages: learnable languages for the picker
// plus every native-language option.
func (s *Service) Languages(ctx context.Context) (api.LanguageList, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT code, english_name, native_name, script, direction, is_learnable, is_native_option, coalesce(launch_order, 0)
		  FROM languages
		 WHERE is_learnable OR is_native_option
		 ORDER BY launch_order NULLS LAST, english_name`)
	if err != nil {
		return api.LanguageList{}, err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (api.Language, error) {
		var l api.Language
		var dir string
		err := r.Scan(&l.Code, &l.EnglishName, &l.NativeName, &l.Script, &dir, &l.IsLearnable, &l.IsNativeOption, &l.LaunchOrder)
		l.Direction = api.LanguageDirection(dir)
		return l, err
	})
	return api.LanguageList{Items: items}, err
}

// Register answers POST /v1/sessions/anonymous. A known install ID gets a
// new token for its existing user, keeping its languages.
func (s *Service) Register(ctx context.Context, req api.AnonymousSessionRequest) (api.SessionResponse, error) {
	if req.BirthYear > s.now().UTC().Year() || req.BirthYear < 1900 {
		return api.SessionResponse{}, invalid("birth_year is out of range")
	}
	band, ok := AgeBand(req.BirthYear, s.now().UTC().Year())
	if !ok {
		return api.SessionResponse{}, errAgeBlocked // nothing is stored
	}
	if len(req.InstallId) < 16 || len(req.InstallId) > 64 {
		return api.SessionResponse{}, invalid("install_id must be 16 to 64 characters")
	}

	var resp api.SessionResponse
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `UPDATE users SET last_seen_at = now() WHERE install_id = $1 RETURNING id`, req.InstallId).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			id, err = s.createUser(ctx, tx, req, band)
		}
		if err != nil {
			return err
		}
		token, hash, err := auth.NewToken()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, now() + make_interval(secs => $3))`,
			hash, id, auth.TokenTTL.Seconds()); err != nil {
			return err
		}
		u, err := loadUser(ctx, tx, id)
		resp = api.SessionResponse{Token: token, User: u}
		return err
	})
	return resp, err
}

func (s *Service) createUser(ctx context.Context, tx pgx.Tx, req api.AnonymousSessionRequest, band string) (uuid.UUID, error) {
	if len(req.Learning) == 0 {
		return uuid.Nil, invalid("learning must list at least one language")
	}
	if err := checkLanguage(ctx, tx, req.UiLanguage, false); err != nil {
		return uuid.Nil, err
	}
	if err := checkLanguage(ctx, tx, req.NativeLanguage, false); err != nil {
		return uuid.Nil, err
	}
	id := uuid.Must(uuid.NewV7())
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, kind, install_id, age_band, ui_language, native_language)
		VALUES ($1, 'anonymous', $2, $3, $4, $5)`,
		id, req.InstallId, band, req.UiLanguage, req.NativeLanguage); err != nil {
		if isUniqueViolation(err) { // another request registered this install first
			return uuid.Nil, httpx.E(http.StatusConflict, "conflict", "install is being registered, retry")
		}
		return uuid.Nil, err
	}
	seen := map[string]bool{}
	for i, l := range req.Learning {
		if seen[l.Language] {
			return uuid.Nil, invalid("duplicate learning language: " + l.Language)
		}
		seen[l.Language] = true
		if err := checkLevel(l.Level); err != nil {
			return uuid.Nil, err
		}
		if err := checkLanguage(ctx, tx, l.Language, true); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO learner_languages (user_id, language, level, is_active) VALUES ($1, $2, $3, $4)`,
			id, l.Language, l.Level, i == 0); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

// Me answers GET /v1/me.
func (s *Service) Me(ctx context.Context) (api.User, error) {
	cu, err := currentUser(ctx)
	if err != nil {
		return api.User{}, err
	}
	return loadUser(ctx, s.DB, cu.ID)
}

// UpdateMe answers PATCH /v1/me.
func (s *Service) UpdateMe(ctx context.Context, p api.UserPatch) (api.User, error) {
	cu, err := currentUser(ctx)
	if err != nil {
		return api.User{}, err
	}
	var u api.User
	err = pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		if p.UiLanguage != nil {
			if err := checkLanguage(ctx, tx, *p.UiLanguage, false); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET ui_language = $2 WHERE id = $1`, cu.ID, *p.UiLanguage); err != nil {
				return err
			}
		}
		if p.NativeLanguage != nil {
			if err := checkLanguage(ctx, tx, *p.NativeLanguage, false); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET native_language = $2 WHERE id = $1`, cu.ID, *p.NativeLanguage); err != nil {
				return err
			}
		}
		u, err = loadUser(ctx, tx, cu.ID)
		return err
	})
	return u, err
}

// PutLanguage answers PUT /v1/me/languages/{code}: add a language or change
// its level; is_active=true makes it the feed language.
func (s *Service) PutLanguage(ctx context.Context, code string, p api.LearningLanguagePut) (api.User, error) {
	cu, err := currentUser(ctx)
	if err != nil {
		return api.User{}, err
	}
	if err := checkLevel(p.Level); err != nil {
		return api.User{}, err
	}
	var u api.User
	err = pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		if err := lockUser(ctx, tx, cu.ID); err != nil {
			return err
		}
		if err := checkLanguage(ctx, tx, code, true); err != nil {
			return err
		}
		activate := p.IsActive != nil && *p.IsActive
		if activate {
			if _, err := tx.Exec(ctx, `UPDATE learner_languages SET is_active = false WHERE user_id = $1 AND language <> $2`, cu.ID, code); err != nil {
				return err
			}
		}
		// A user's first language is always active.
		if _, err := tx.Exec(ctx, `
			INSERT INTO learner_languages (user_id, language, level, is_active)
			VALUES ($1, $2, $3, $4 OR NOT EXISTS (SELECT 1 FROM learner_languages WHERE user_id = $1 AND is_active))
			ON CONFLICT (user_id, language) DO UPDATE
			   SET level = EXCLUDED.level,
			       is_active = learner_languages.is_active OR $4`,
			cu.ID, code, p.Level, activate); err != nil {
			return err
		}
		u, err = loadUser(ctx, tx, cu.ID)
		return err
	})
	return u, err
}

// DeleteLanguage answers DELETE /v1/me/languages/{code}. The last language
// can't be removed; removing the active one activates the next by launch
// order.
func (s *Service) DeleteLanguage(ctx context.Context, code string) (api.User, error) {
	cu, err := currentUser(ctx)
	if err != nil {
		return api.User{}, err
	}
	var u api.User
	err = pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		if err := lockUser(ctx, tx, cu.ID); err != nil {
			return err
		}
		var total int
		var wasActive *bool
		if err := tx.QueryRow(ctx, `
			SELECT count(*), bool_or(is_active) FILTER (WHERE language = $2)
			  FROM learner_languages WHERE user_id = $1`, cu.ID, code).Scan(&total, &wasActive); err != nil {
			return err
		}
		if wasActive == nil {
			return httpx.E(http.StatusNotFound, "not_found", "not learning "+code)
		}
		if total <= 1 {
			return errLastLanguage
		}
		if _, err := tx.Exec(ctx, `DELETE FROM learner_languages WHERE user_id = $1 AND language = $2`, cu.ID, code); err != nil {
			return err
		}
		if *wasActive {
			if _, err := tx.Exec(ctx, `
				UPDATE learner_languages SET is_active = true
				 WHERE user_id = $1 AND language = (
					SELECT ll.language FROM learner_languages ll JOIN languages l ON l.code = ll.language
					 WHERE ll.user_id = $1 ORDER BY l.launch_order NULLS LAST, l.code LIMIT 1)`, cu.ID); err != nil {
				return err
			}
		}
		u, err = loadUser(ctx, tx, cu.ID)
		return err
	})
	return u, err
}

// DeleteMe answers DELETE /v1/me: the user and everything tied to it go;
// events stay with user_id set to null. Its tokens stop working.
func (s *Service) DeleteMe(ctx context.Context) error {
	cu, err := currentUser(ctx)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM users WHERE id = $1`, cu.ID)
	return err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadUser(ctx context.Context, q querier, id uuid.UUID) (api.User, error) {
	var u api.User
	var band string
	err := q.QueryRow(ctx, `SELECT id, age_band, ui_language, native_language FROM users WHERE id = $1`, id).
		Scan(&u.Id, &band, &u.UiLanguage, &u.NativeLanguage)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, httpx.E(http.StatusUnauthorized, "unauthorized", "user no longer exists")
	}
	if err != nil {
		return api.User{}, err
	}
	u.AgeBand = api.UserAgeBand(band)
	rows, err := q.Query(ctx, `
		SELECT ll.language, ll.level, ll.is_active
		  FROM learner_languages ll JOIN languages l ON l.code = ll.language
		 WHERE ll.user_id = $1
		 ORDER BY ll.is_active DESC, l.launch_order NULLS LAST, l.code`, id)
	if err != nil {
		return api.User{}, err
	}
	u.Learning, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (api.LearningLanguage, error) {
		var l api.LearningLanguage
		err := r.Scan(&l.Language, &l.Level, &l.IsActive)
		return l, err
	})
	if u.Learning == nil {
		u.Learning = []api.LearningLanguage{}
	}
	return u, err
}

func lockUser(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, id)
	return err
}

func checkLanguage(ctx context.Context, q querier, code string, mustBeLearnable bool) error {
	var learnable, native bool
	err := q.QueryRow(ctx, `SELECT is_learnable, is_native_option FROM languages WHERE code = $1`, code).Scan(&learnable, &native)
	if errors.Is(err, pgx.ErrNoRows) || (mustBeLearnable && !learnable) {
		return unknownLanguage(code)
	}
	return err
}

func checkLevel(level int) error {
	if level < 1 || level > 6 {
		return invalid("level must be 1 to 6")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
