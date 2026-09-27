// Свет: световой день из Open-Meteo и сессии лампы по расписанию. Порт app/services/light.py.
package light

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/models"
	"poliv/internal/summary"
)

const geocodeURL = "https://geocoding-api.open-meteo.com/v1/search"
const forecastURL = "https://api.open-meteo.com/v1/forecast"
const refreshAfter = 3 * time.Hour

var httpClient = &http.Client{Timeout: 10 * time.Second}

// FetchDays — перезапрашиваемый извне для подмены в тестах.
var FetchDays = fetchDays

type DayRow struct {
	Day           summary.Date
	Sunrise       *time.Time
	Sunset        *time.Time
	DaylightHours float64
	SunshineHours float64
}

func GetJSON(endpoint string, params map[string]string) (map[string]any, error) {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	resp, err := httpClient.Get(endpoint + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

type Place struct {
	Name      string
	Region    *string
	Country   *string
	Latitude  float64
	Longitude float64
}

func Geocode(query string) ([]Place, error) {
	data, err := GetJSON(geocodeURL, map[string]string{
		"name": query, "count": "8", "language": "ru", "format": "json",
	})
	if err != nil {
		return nil, err
	}
	var out []Place
	results, _ := data["results"].([]any)
	for _, r := range results {
		rm, _ := r.(map[string]any)
		p := Place{}
		p.Name, _ = rm["name"].(string)
		if v, ok := rm["admin1"].(string); ok {
			p.Region = &v
		}
		if v, ok := rm["country"].(string); ok {
			p.Country = &v
		}
		p.Latitude, _ = rm["latitude"].(float64)
		p.Longitude, _ = rm["longitude"].(float64)
		out = append(out, p)
	}
	return out, nil
}

func fetchDays(latitude, longitude float64, tz *time.Location) ([]DayRow, error) {
	data, err := GetJSON(forecastURL, map[string]string{
		"latitude":       strconv.FormatFloat(latitude, 'f', 3, 64),
		"longitude":      strconv.FormatFloat(longitude, 'f', 3, 64),
		"daily":          "sunrise,sunset,daylight_duration,sunshine_duration",
		"timezone":       tz.String(),
		"past_days":      "2",
		"forecast_days":  "3",
	})
	if err != nil {
		return nil, err
	}
	daily, _ := data["daily"].(map[string]any)
	if daily == nil {
		return nil, fmt.Errorf("no daily data")
	}
	times, _ := daily["time"].([]any)
	sunrise, _ := daily["sunrise"].([]any)
	sunset, _ := daily["sunset"].([]any)
	daylight, _ := daily["daylight_duration"].([]any)
	sunshine, _ := daily["sunshine_duration"].([]any)
	out := make([]DayRow, 0, len(times))
	for i := range times {
		dayStr, _ := times[i].(string)
		day, err := time.ParseInLocation("2006-01-02", dayStr, time.UTC)
		if err != nil {
			continue
		}
		row := DayRow{Day: day}
		if v, ok := sunrise[i].(string); ok {
			if t, err := time.ParseInLocation("2006-01-02T15:04", v, tz); err == nil {
				row.Sunrise = &t
			}
		}
		if v, ok := sunset[i].(string); ok {
			if t, err := time.ParseInLocation("2006-01-02T15:04", v, tz); err == nil {
				row.Sunset = &t
			}
		}
		row.DaylightHours = round2(daylight[i])
		row.SunshineHours = round2(sunshine[i])
		out = append(out, row)
	}
	return out, nil
}

func round2(v any) float64 {
	switch n := v.(type) {
	case float64:
		return math.Round(n*100) / 100
	case int:
		return math.Round(float64(n)*100) / 100
	case nil:
		return 0
	}
	return 0
}

// SyncDaylight — обновить свет по городу учётки. True — ходили в Open-Meteo.
func SyncDaylight(ctx context.Context, pool *pgxpool.Pool, s *models.UserSettings, tz *time.Location, force bool) (bool, error) {
	if s.Latitude == nil || s.Longitude == nil {
		return false, nil
	}
	now := time.Now().In(tz)
	today := summary.LocalDate(now, tz)
	var fetchedAt time.Time
	err := pool.QueryRow(ctx, `SELECT fetched_at FROM daylight_days WHERE user_id=$1 AND day=$2`,
		s.UserID, today).Scan(&fetchedAt)
	if err != nil && err != pgx.ErrNoRows {
		return false, err
	}
	if !force && err == nil && now.Sub(fetchedAt) < refreshAfter {
		return false, nil
	}
	rows, err := FetchDays(*s.Latitude, *s.Longitude, tz)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if _, err := pool.Exec(ctx, `INSERT INTO daylight_days
			(user_id, day, sunrise, sunset, daylight_hours, sunshine_hours, fetched_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (user_id, day) DO UPDATE SET sunrise=EXCLUDED.sunrise, sunset=EXCLUDED.sunset,
			daylight_hours=EXCLUDED.daylight_hours, sunshine_hours=EXCLUDED.sunshine_hours, fetched_at=EXCLUDED.fetched_at`,
			s.UserID, r.Day, r.Sunrise, r.Sunset, r.DaylightHours, r.SunshineHours, now); err != nil {
			return false, err
		}
	}
	return true, nil
}

func TodayDaylight(ctx context.Context, pool *pgxpool.Pool, userID int, tz *time.Location) (*models.DaylightDay, error) {
	today := summary.LocalDate(time.Now().In(tz), tz)
	row := pool.QueryRow(ctx, `SELECT user_id, day, sunrise, sunset, daylight_hours, sunshine_hours, fetched_at
		FROM daylight_days WHERE user_id=$1 AND day=$2`, userID, today)
	var d models.DaylightDay
	err := row.Scan(&d.UserID, &d.Day, &d.Sunrise, &d.Sunset, &d.DaylightHours, &d.SunshineHours, &d.FetchedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func SunshineByDay(ctx context.Context, pool *pgxpool.Pool, userID int, since summary.Date) (map[summary.Date]float64, error) {
	rows, err := pool.Query(ctx, `SELECT day, sunshine_hours FROM daylight_days WHERE user_id=$1 AND day >= $2`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[summary.Date]float64{}
	for rows.Next() {
		var d summary.Date
		var h float64
		if err := rows.Scan(&d, &h); err != nil {
			return nil, err
		}
		out[d] = h
	}
	return out, rows.Err()
}

// ---------- расписание → сессии ----------

type ScheduleInterval = summary.Interval

// ValidateIntervals — сортирует и проверяет интервалы (пересечения / конец раньше начала).
func ValidateIntervals(intervals []ScheduleInterval) ([]ScheduleInterval, error) {
	out := make([]ScheduleInterval, len(intervals))
	copy(out, intervals)
	sortByClock(out)
	for _, iv := range out {
		if !iv.End.After(iv.Start) {
			return nil, fmt.Errorf("Конец интервала должен быть позже начала (через полночь — двумя интервалами)")
		}
	}
	for i := 1; i < len(out); i++ {
		if clockMinute(out[i].Start) < clockMinute(out[i-1].End) {
			return nil, fmt.Errorf("Интервалы пересекаются")
		}
	}
	return out, nil
}

func clockMinute(t time.Time) int { return t.Hour()*60 + t.Minute() }

func sortByClock(ivs []ScheduleInterval) {
	for i := 1; i < len(ivs); i++ {
		for j := i; j > 0 && clockMinute(ivs[j].Start) < clockMinute(ivs[j-1].Start); j-- {
			ivs[j], ivs[j-1] = ivs[j-1], ivs[j]
		}
	}
}

func SchedulesFor(ctx context.Context, pool *pgxpool.Pool, lampID int) ([]models.LampSchedule, error) {
	rows, err := pool.Query(ctx, `SELECT id, user_id, lamp_id, start_time, end_time
		FROM lamp_schedules WHERE lamp_id=$1 ORDER BY start_time`, lampID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.LampSchedule
	for rows.Next() {
		var s models.LampSchedule
		if err := rows.Scan(&s.ID, &s.UserID, &s.LampID, &s.StartTime, &s.EndTime); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// EnsureScheduleSessions — создать на день сессии по расписаниям ламп режима «schedule» (идемпотентно).
func EnsureScheduleSessions(ctx context.Context, pool *pgxpool.Pool, userID int, day summary.Date, tz *time.Location) (int, error) {
	start := summary.LocalMidnight(day, tz)
	end := summary.LocalMidnight(summary.DateAddDays(day, 1), tz)
	rows, err := pool.Query(ctx, `SELECT ls.id, ls.lamp_id, ls.start_time, ls.end_time
		FROM lamp_schedules ls JOIN lamps l ON l.id = ls.lamp_id
		WHERE ls.user_id=$1 AND l.mode='schedule' AND l.archived_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var created int
	var schedRows []models.LampSchedule
	for rows.Next() {
		var s models.LampSchedule
		if err := rows.Scan(&s.ID, &s.LampID, &s.StartTime, &s.EndTime); err != nil {
			return 0, err
		}
		schedRows = append(schedRows, s)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, sch := range schedRows {
		var exists int
		err := pool.QueryRow(ctx, `SELECT 1 FROM lamp_sessions WHERE schedule_id=$1 AND started_at >= $2 AND started_at < $3`,
			sch.ID, start, end).Scan(&exists)
		if err == nil {
			continue
		}
		if err != pgx.ErrNoRows {
			return 0, err
		}
		iv := ScheduleInterval{Start: sch.StartTime, End: sch.EndTime}
		sessions := summary.ScheduleSessionsForDay([]ScheduleInterval{iv}, day, tz)
		for _, ses := range sessions {
			if _, err := pool.Exec(ctx, `INSERT INTO lamp_sessions (user_id, lamp_id, schedule_id, source, started_at, ended_at)
				VALUES ($1,$2,$3,'schedule',$4,$5)`, userID, sch.LampID, sch.ID, ses.Start, ses.End); err != nil {
				return created, err
			}
			created++
		}
	}
	return created, nil
}

// ReplaceSchedule — новое расписание лампы: пересоздаёт сегодняшние сессии, прошлые дни не трогает.
func ReplaceSchedule(ctx context.Context, pool *pgxpool.Pool, userID, lampID int, intervals []ScheduleInterval, tz *time.Location) ([]models.LampSchedule, error) {
	today := summary.LocalDate(time.Now().In(tz), tz)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	oldRows, err := SchedulesFor(ctx, pool, lampID)
	if err != nil {
		return nil, err
	}
	var oldIDs []int
	for _, s := range oldRows {
		oldIDs = append(oldIDs, s.ID)
	}
	if len(oldIDs) > 0 {
		ids := intsToAny(oldIDs)
		if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE schedule_id = ANY($1) AND started_at >= $2`,
			ids, summary.LocalMidnight(today, tz)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM lamp_schedules WHERE id = ANY($1)`, ids); err != nil {
			return nil, err
		}
	}
	for _, iv := range intervals {
		if _, err := tx.Exec(ctx, `INSERT INTO lamp_schedules (user_id, lamp_id, start_time, end_time) VALUES ($1,$2,$3,$4)`,
			userID, lampID, iv.Start, iv.End); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if _, err := EnsureScheduleSessions(ctx, pool, userID, today, tz); err != nil {
		return nil, err
	}
	return SchedulesFor(ctx, pool, lampID)
}

func intsToAny(ids []int) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// SyncAll — раз в 30 минут: свет по городу для всех учёток.
func SyncAll(ctx context.Context, pool *pgxpool.Pool, tz *time.Location) error {
	rows, err := pool.Query(ctx, `SELECT id FROM users WHERE blocked_at IS NULL`)
	if err != nil {
		return err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		s, err := GetSettings(ctx, pool, id)
		if err != nil {
			continue
		}
		if s != nil {
			_, _ = SyncDaylight(ctx, pool, s, tz, false)
		}
	}
	return nil
}

func GetSettings(ctx context.Context, pool *pgxpool.Pool, userID int) (*models.UserSettings, error) {
	var s models.UserSettings
	err := pool.QueryRow(ctx, `SELECT user_id, current_season::text, notify_days_ahead, location_name,
		latitude, longitude, yandex_token, yandex_token_invalid FROM user_settings WHERE user_id=$1`, userID).Scan(
		&s.UserID, &s.CurrentSeason, &s.NotifyDaysAhead, &s.LocationName, &s.Latitude, &s.Longitude, &s.YandexToken, &s.YandexTokenInvalid)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
