package users

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/db"
	"poliv/internal/models"
	"poliv/internal/passwords"
)

const PlaceholderEmail = "owner@localhost.invalid"

var DefaultFertilizers = []struct {
	Name                       string
	IntervalDaysActiveSeason   int
	IntervalDaysDormantSeason  int
}{
	{"Lomonosoff", 14, 30},
	{"Bona Forte", 14, 30},
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// FindByEmail — пользователь по почте (без регистра). Как find_by_email в Python.
func FindByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*models.User, error) {
	email = NormalizeEmail(email)
	rows, err := pool.Query(ctx, `SELECT id, email, password_hash, is_admin, blocked_at, token_version, created_at
		FROM users WHERE lower(email) = $1`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanUser(rows)
}

func scanUser(row pgx.Row) (*models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.BlockedAt, &u.TokenVersion, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser — учётка + настройки + стартовые удобрения одной транзакцией.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, email, password string, isAdmin bool) (*models.User, error) {
	email = NormalizeEmail(email)
	if err := passwords.CheckPassword(password); err != nil {
		return nil, err
	}
	existing, err := FindByEmail(ctx, pool, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailExists
	}
	hash, err := passwords.HashPassword(password)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id int
	if err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, is_admin)
		VALUES ($1, $2, $3) RETURNING id`, email, hash, isAdmin).Scan(&id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_settings (user_id) VALUES ($1)`, id); err != nil {
		return nil, err
	}
	for _, f := range DefaultFertilizers {
		if _, err := tx.Exec(ctx, `INSERT INTO fertilizer_types
			(user_id, name, interval_days_active_season, interval_days_dormant_season)
			VALUES ($1, $2, $3, $4)`, id, f.Name, f.IntervalDaysActiveSeason, f.IntervalDaysDormantSeason); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return FindByEmail(ctx, pool, email)
}

// SetPassword — хэш + token_version++ (разлогинивает все устройства).
func SetPassword(ctx context.Context, pool *pgxpool.Pool, user *models.User, password string) error {
	if err := passwords.CheckPassword(password); err != nil {
		return err
	}
	hash, err := passwords.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `UPDATE users SET password_hash = $1, token_version = token_version + 1 WHERE id = $2`, hash, user.ID)
	return err
}

func SetBlocked(ctx context.Context, pool *pgxpool.Pool, user *models.User, blocked bool) error {
	if blocked {
		if user.BlockedAt == nil {
			_, err := pool.Exec(ctx, `UPDATE users SET blocked_at = now(), token_version = token_version + 1 WHERE id = $1`, user.ID)
			return err
		}
		return nil
	}
	_, err := pool.Exec(ctx, `UPDATE users SET blocked_at = NULL WHERE id = $1`, user.ID)
	return err
}

// GetUserSettings — настройки учётки; если их нет — создаёт пустые (как в Python).
func GetUserSettings(ctx context.Context, pool *pgxpool.Pool, userID int) (*models.UserSettings, error) {
	var s models.UserSettings
	err := pool.QueryRow(ctx, `SELECT user_id, current_season::text, notify_days_ahead, location_name,
		latitude, longitude, yandex_token, yandex_token_invalid
		FROM user_settings WHERE user_id = $1`, userID).Scan(
		&s.UserID, &s.CurrentSeason, &s.NotifyDaysAhead, &s.LocationName, &s.Latitude, &s.Longitude, &s.YandexToken, &s.YandexTokenInvalid)
	if err == pgx.ErrNoRows {
		if _, err := pool.Exec(ctx, `INSERT INTO user_settings (user_id) VALUES ($1)`, userID); err != nil {
			return nil, err
		}
		return GetUserSettings(ctx, pool, userID)
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

var ErrEmailExists = &emailError{}

type emailError struct{}

func (e *emailError) Error() string { return "Учётка с такой почтой уже есть" }

var _ = db.NowUTC

func GetByID(ctx context.Context, pool *pgxpool.Pool, id int) (*models.User, error) {
	row := pool.QueryRow(ctx, `SELECT id, email, password_hash, is_admin, blocked_at, token_version, created_at
		FROM users WHERE id=$1`, id)
	u, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}
