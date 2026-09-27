// Лампы: растения под лампой (периоды привязки), сессии растения, переключение,
// досветка до нормы и розетка в Умном доме Яндекса. Порт app/services/lamps.py.
package lamps

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/light"
	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/secretbox"
	"poliv/internal/summary"
	"poliv/internal/yandex"
)

// querier — общий интерфейс для pool и tx: чтение/запись в одной транзакции.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const PauseMinutes = 10

var ErrLampNotOn = errors.New("Лампа не горит — её не нужно ставить на паузу")
var ErrLampNotPaused = errors.New("Лампа не на паузе")

// Service — лампы и розетки. Zone — календарная зона, Secret — JWT_SECRET для токена Яндекса.
type Service struct {
	Pool   *pgxpool.Pool
	Zone   *time.Location
	Secret string

	// Функции Яндекса — можно подменить в тестах (в сеть не ходим).
	YandexSetOn      func(token, deviceID string, on bool) error
	YandexListDevices func(token string) ([]yandex.Device, error)
}

func New(pool *pgxpool.Pool, zone *time.Location, secret string) *Service {
	return &Service{Pool: pool, Zone: zone, Secret: secret}
}

func (s *Service) yandexSetOn(token, deviceID string, on bool) error {
	if s.YandexSetOn != nil {
		return s.YandexSetOn(token, deviceID, on)
	}
	return yandex.SetOn(token, deviceID, on)
}

func (s *Service) yandexListDevices(token string) ([]yandex.Device, error) {
	if s.YandexListDevices != nil {
		return s.YandexListDevices(token)
	}
	return yandex.ListDevices(token)
}

// ---------- растения под лампой ----------

func (s *Service) CurrentLampID(ctx context.Context, plantID int) (*int, error) {
	var id *int
	err := s.Pool.QueryRow(ctx, `SELECT lamp_id FROM plant_lamps WHERE plant_id=$1 AND ended_at IS NULL`, plantID).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (s *Service) PlantIDs(ctx context.Context, q querier, lampID int) ([]int, error) {
	rows, err := q.Query(ctx, `SELECT plant_id FROM plant_lamps WHERE lamp_id=$1 AND ended_at IS NULL ORDER BY plant_id`, lampID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var pid int
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out = append(out, pid)
	}
	return out, rows.Err()
}

// assign — поставить растение под лампу (nil — без лампы). Внутри транзакции.
func assign(ctx context.Context, tx pgx.Tx, plantID int, lampID *int, now time.Time) error {
	var curID *int
	err := tx.QueryRow(ctx, `SELECT lamp_id FROM plant_lamps WHERE plant_id=$1 AND ended_at IS NULL`, plantID).Scan(&curID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil && curID != nil && lampID != nil && *curID == *lampID {
		return nil
	}
	if err == nil {
		if _, err := tx.Exec(ctx, `UPDATE plant_lamps SET ended_at=$1 WHERE plant_id=$2 AND ended_at IS NULL`, now, plantID); err != nil {
			return err
		}
	}
	if lampID != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO plant_lamps (plant_id, lamp_id, started_at) VALUES ($1,$2,$3)`,
			plantID, *lampID, now); err != nil {
			return err
		}
	}
	return nil
}

// SetPlants — полная замена растений лампы. Владелец проверен вызывающим.
func (s *Service) SetPlants(ctx context.Context, lamp *models.Lamp, ids []int, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	cur, err := s.PlantIDs(ctx, s.Pool, lamp.ID)
	if err != nil {
		return err
	}
	in := map[int]bool{}
	for _, id := range ids {
		in[id] = true
	}
	for _, pid := range cur {
		if !in[pid] {
			if err := assign(ctx, tx, pid, nil, now); err != nil {
				return err
			}
		}
	}
	for _, pid := range ids {
		if err := assign(ctx, tx, pid, intPtr(lamp.ID), now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ---------- сессии растения ----------

type PlantSession struct {
	ID          int
	LampID      int
	LampName    string
	StartedAt   time.Time
	EndedAt     *time.Time
}

// PlantSessions — сессии ламп, под которыми растение стояло, обрезанные по периодам привязки.
func (s *Service) plantSessionsRaw(ctx context.Context, q querier, plantID int, since *time.Time) ([]PlantSession, error) {
	// периоды
	type period struct{ start time.Time; end *time.Time }
	periods := map[int][]period{}
	rows, err := q.Query(ctx, `SELECT lamp_id, started_at, ended_at FROM plant_lamps WHERE plant_id=$1`, plantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var lid int
		var p period
		if err := rows.Scan(&lid, &p.start, &p.end); err != nil {
			rows.Close()
			return nil, err
		}
		periods[lid] = append(periods[lid], p)
	}
	rows.Close()
	if len(periods) == 0 {
		return nil, nil
	}
	// сессии ламп, что были под растением
	var lampIDs []int
	for lid := range periods {
		lampIDs = append(lampIDs, lid)
	}
	qstr := `SELECT ls.id, ls.lamp_id, l.name, ls.started_at, ls.ended_at FROM lamp_sessions ls JOIN lamps l ON l.id=ls.lamp_id WHERE ls.lamp_id = ANY($1)`
	args := []any{intsToAny(lampIDs)}
	if since != nil {
		qstr += ` AND (ls.ended_at IS NULL OR ls.ended_at >= $2)`
		args = append(args, *since)
	}
	rows, err = q.Query(ctx, qstr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlantSession
	for rows.Next() {
		var sid, lid int
		var name string
		var st time.Time
		var en *time.Time
		if err := rows.Scan(&sid, &lid, &name, &st, &en); err != nil {
			return nil, err
		}
		prds := periods[lid]
		var pls []summary.Period
		for _, p := range prds {
			pls = append(pls, summary.Period{Start: p.start, End: p.end})
		}
		for _, seg := range summary.ClipSession(st, en, pls) {
			out = append(out, PlantSession{ID: sid, LampID: lid, LampName: name, StartedAt: seg.Start, EndedAt: seg.End})
		}
	}
	return out, rows.Err()
}

// PlantSessions — сессии ламп, под которыми растение стояло, обрезанные по периодам привязки.
func (s *Service) PlantSessions(ctx context.Context, q querier, plantID int, since *time.Time) ([]PlantSession, error) {
	return s.plantSessionsRaw(ctx, q, plantID, since)
}

func AsRules(ps []PlantSession) []summary.Session {
	var out []summary.Session
	for _, p := range ps {
		out = append(out, summary.Session{Start: p.StartedAt, End: p.EndedAt})
	}
	return out
}

func (s *Service) coveringSession(ctx context.Context, lampID int, now time.Time) (*models.LampSession, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE lamp_id=$1 AND started_at <= $2 AND (ended_at IS NULL OR ended_at > $2)
		ORDER BY (ended_at IS NULL) DESC, started_at DESC LIMIT 1`, lampID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanSession(rows)
}

func (s *Service) coveringSessions(ctx context.Context, lampID int, now time.Time) ([]*models.LampSession, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE lamp_id=$1 AND started_at <= $2 AND (ended_at IS NULL OR ended_at > $2)
		ORDER BY (ended_at IS NULL) DESC, started_at DESC`, lampID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.LampSession
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

func scanSession(row pgx.Row) (*models.LampSession, error) {
	var s models.LampSession
	var source string
	err := row.Scan(&s.ID, &s.UserID, &s.LampID, &source, &s.StartedAt, &s.EndedAt, &s.PlannedHours, &s.ScheduleID, &s.AfterPause)
	if err != nil {
		return nil, err
	}
	s.Source = source
	return &s, nil
}

// ---------- переключение кнопкой ----------

type Toggled struct {
	IsOn             bool
	Session          *models.LampSession
	PreviousEndedAt  *time.Time
}

func (s *Service) Toggle(ctx context.Context, lamp *models.Lamp, now time.Time) (*Toggled, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if lamp.PausedUntil != nil {
		if lamp.PausedUntil.After(now) {
			tails, err := s.tailSessions(ctx, tx, lamp)
			if err != nil {
				return nil, err
			}
			for _, t := range tails {
				if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE id=$1`, t.ID); err != nil {
					return nil, err
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE lamps SET paused_until=NULL WHERE id=$1`, lamp.ID); err != nil {
			return nil, err
		}
	}
	currents, err := s.coveringSessionsTx(ctx, tx, lamp.ID, now)
	if err != nil {
		return nil, err
	}
	var current *models.LampSession
	var result Toggled
	if len(currents) > 0 {
		current = currents[0]
		result = Toggled{IsOn: false, Session: current, PreviousEndedAt: current.EndedAt}
		for _, c := range currents {
			if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET ended_at=$1 WHERE id=$2`, now, c.ID); err != nil {
				return nil, err
			}
		}
	} else {
		var id int
		if err := tx.QueryRow(ctx, `INSERT INTO lamp_sessions (user_id, lamp_id, source, started_at) VALUES ($1,$2,'manual',$3) RETURNING id`,
			lamp.UserID, lamp.ID, now).Scan(&id); err != nil {
			return nil, err
		}
		current = &models.LampSession{ID: id, UserID: lamp.UserID, LampID: lamp.ID, Source: "manual", StartedAt: now, PlannedHours: 12}
		result = Toggled{IsOn: true, Session: current}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	fresh, err := s.getSession(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	result.Session = fresh
	if err := s.SyncPlug(ctx, lamp, now); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) getSession(ctx context.Context, id int) (*models.LampSession, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE id=$1`, id)
	ss, err := scanSession(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return ss, err
}

func (s *Service) coveringSessionsTx(ctx context.Context, tx pgx.Tx, lampID int, now time.Time) ([]*models.LampSession, error) {
	rows, err := tx.Query(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE lamp_id=$1 AND started_at <= $2 AND (ended_at IS NULL OR ended_at > $2)
		ORDER BY (ended_at IS NULL) DESC, started_at DESC`, lampID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.LampSession
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// ---------- пауза лампы ----------

func (s *Service) tailSessions(ctx context.Context, tx pgx.Tx, lamp *models.Lamp) ([]*models.LampSession, error) {
	if lamp.PausedUntil == nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE lamp_id=$1 AND after_pause AND started_at=$2`, lamp.ID, *lamp.PausedUntil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.LampSession
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

func (s *Service) clampAutoEnd(end time.Time, lamp *models.Lamp) time.Time {
	limitRule := summary.LocalDate(end.In(s.Zone), s.Zone)
	limit := summary.Combine(limitRule, lamp.EveningNotAfter.Hour(), lamp.EveningNotAfter.Minute(), s.Zone)
	if end.Before(limit) {
		return end
	}
	return limit
}

func (s *Service) cutSession(ctx context.Context, tx pgx.Tx, lamp *models.Lamp, current *models.LampSession, now time.Time) error {
	oldEnd := current.EndedAt
	if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET ended_at=$1 WHERE id=$2`, now, current.ID); err != nil {
		return err
	}
	pu := *lamp.PausedUntil
	switch current.Source {
	case "auto":
		var newEnd *time.Time
		if oldEnd != nil {
			nc := s.clampAutoEnd(oldEnd.Add(PauseMinutes*time.Minute), lamp)
			newEnd = &nc
		}
		if newEnd != nil && newEnd.After(pu) {
			if _, err := tx.Exec(ctx, `INSERT INTO lamp_sessions (user_id, lamp_id, source, started_at, ended_at, after_pause)
				VALUES ($1,$2,'auto',$3,$4,true)`, lamp.UserID, lamp.ID, pu, *newEnd); err != nil {
				return err
			}
		}
	case "schedule":
		if oldEnd != nil && oldEnd.After(pu) {
			if _, err := tx.Exec(ctx, `INSERT INTO lamp_sessions (user_id, lamp_id, source, schedule_id, started_at, ended_at, after_pause)
				VALUES ($1,$2,'schedule',$3,$4,$5,true)`, lamp.UserID, lamp.ID, current.ScheduleID, pu, *oldEnd); err != nil {
				return err
			}
		}
	default: // manual — открытая сессия, хвост горит с paused_until
		if _, err := tx.Exec(ctx, `INSERT INTO lamp_sessions (user_id, lamp_id, source, started_at, after_pause)
			VALUES ($1,$2,'manual',$3,true)`, lamp.UserID, lamp.ID, pu); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) extendPause(ctx context.Context, tx pgx.Tx, lamp *models.Lamp) error {
	tails, err := s.tailSessions(ctx, tx, lamp)
	if err != nil {
		return err
	}
	oldPause := *lamp.PausedUntil
	newPause := oldPause.Add(PauseMinutes * time.Minute)
	if _, err := tx.Exec(ctx, `UPDATE lamps SET paused_until=$1 WHERE id=$2`, newPause, lamp.ID); err != nil {
		return err
	}
	lamp.PausedUntil = &newPause
	for _, tail := range tails {
		if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET started_at=$1 WHERE id=$2`, newPause, tail.ID); err != nil {
			return err
		}
		if tail.Source == "auto" && tail.EndedAt != nil {
			nc := s.clampAutoEnd(tail.EndedAt.Add(PauseMinutes*time.Minute), lamp)
			if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET ended_at=$1 WHERE id=$2`, nc, tail.ID); err != nil {
				return err
			}
		} else if tail.Source == "schedule" && tail.EndedAt != nil && !newPause.Before(*tail.EndedAt) {
			// расписание жёсткое — пауза съела хвост целиком
			if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE id=$1`, tail.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Pause(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if lamp.PausedUntil != nil && lamp.PausedUntil.After(now) {
		if err := s.extendPause(ctx, tx, lamp); err != nil {
			return err
		}
	} else {
		currents, err := s.coveringSessionsTx(ctx, tx, lamp.ID, now)
		if err != nil {
			return err
		}
		if len(currents) == 0 {
			return ErrLampNotOn
		}
		pu := now.Add(PauseMinutes * time.Minute)
		if _, err := tx.Exec(ctx, `UPDATE lamps SET paused_until=$1 WHERE id=$2`, pu, lamp.ID); err != nil {
			return err
		}
		lamp.PausedUntil = &pu
		for _, c := range currents {
			if err := s.cutSession(ctx, tx, lamp, c, now); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.SyncPlug(ctx, lamp, now)
}

func (s *Service) Resume(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	if lamp.PausedUntil == nil || !lamp.PausedUntil.After(now) {
		return ErrLampNotPaused
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tails, err := s.tailSessions(ctx, tx, lamp)
	if err != nil {
		return err
	}
	leftover := lamp.PausedUntil.Sub(now)
	if _, err := tx.Exec(ctx, `UPDATE lamps SET paused_until=NULL WHERE id=$1`, lamp.ID); err != nil {
		return err
	}
	for _, tail := range tails {
		if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET started_at=$1 WHERE id=$2`, now, tail.ID); err != nil {
			return err
		}
		if tail.Source == "auto" && tail.EndedAt != nil {
			ne := tail.EndedAt.Add(-leftover)
			if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET ended_at=$1 WHERE id=$2`, ne, tail.ID); err != nil {
				return err
			}
		} else if tail.Source == "schedule" && tail.EndedAt != nil && !now.Before(*tail.EndedAt) {
			if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE id=$1`, tail.ID); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.SyncPlug(ctx, lamp, now)
}

// ---------- досветка до нормы ----------

func (s *Service) autoToday(ctx context.Context, q querier, lamp *models.Lamp, now time.Time) ([]*models.LampSession, error) {
	start := summary.LocalMidnight(summary.LocalDate(now, s.Zone), s.Zone)
	end := summary.LocalMidnight(summary.DateAddDays(summary.LocalDate(now, s.Zone), 1), s.Zone)
	rows, err := q.Query(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE lamp_id=$1 AND source='auto' AND started_at >= $2 AND started_at < $3 ORDER BY started_at`,
		lamp.ID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.LampSession
	for rows.Next() {
		ss, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

func (s *Service) worstDeficit(ctx context.Context, q querier, ids []int, natural float64, now time.Time) (float64, error) {
	day := summary.LocalDate(now, s.Zone)
	since := summary.LocalMidnight(day, s.Zone)
	var deficits []float64
	if len(ids) == 0 {
		return summary.LampNeed(deficits), nil
	}
	type plantTarget struct{ id int; target float64 }
	var plants []plantTarget
	rows, err := q.Query(ctx, `SELECT id, light_target_hours FROM plants WHERE id = ANY($1)`, intsToAny(ids))
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var pid int
		var target float64
		if err := rows.Scan(&pid, &target); err != nil {
			rows.Close()
			return 0, err
		}
		plants = append(plants, plantTarget{pid, target})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, pt := range plants {
		ps, err := s.PlantSessions(ctx, q, pt.id, &since)
		if err != nil {
			return 0, err
		}
		hours := summary.LampHoursInDay(AsRules(ps), day, now, s.Zone, true)
		nat := &natural
		deficits = append(deficits, summary.LightStateOf(pt.target, nat, hours).DeficitHours)
	}
	return summary.LampNeed(deficits), nil
}

func (s *Service) addAuto(ctx context.Context, tx pgx.Tx, lamp *models.Lamp, window *summary.Session, now time.Time) error {
	if window == nil {
		return nil
	}
	start := window.Start
	if start.Before(now) {
		start = now
	}
	if window.End.After(start) {
		_, err := tx.Exec(ctx, `INSERT INTO lamp_sessions (user_id, lamp_id, source, started_at, ended_at)
			VALUES ($1,$2,'auto',$3,$4)`, lamp.UserID, lamp.ID, start, *window.End)
		return err
	}
	return nil
}

// ReplanAuto — досветка на сегодня: будущие части пересоздаются, начавшиеся и хвосты паузы — не трогаются.
func (s *Service) ReplanAuto(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	day := summary.LocalDate(now, s.Zone)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	autos, err := s.autoToday(ctx, tx, lamp, now)
	if err != nil {
		return err
	}
	for _, a := range autos {
		if a.StartedAt.After(now) && !a.AfterPause {
			if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE id=$1`, a.ID); err != nil {
				return err
			}
		}
	}
	kept := map[int]bool{}
	for _, a := range autos {
		if !a.StartedAt.After(now) || a.AfterPause {
			kept[a.ID] = true
		}
	}
	daylight := &models.DaylightDay{}
	err = tx.QueryRow(ctx, `SELECT user_id, day, sunrise, sunset, daylight_hours, sunshine_hours, fetched_at
		FROM daylight_days WHERE user_id=$1 AND day=$2`, lamp.UserID, day).Scan(
		&daylight.UserID, &daylight.Day, &daylight.Sunrise, &daylight.Sunset, &daylight.DaylightHours, &daylight.SunshineHours, &daylight.FetchedAt)
	if err == pgx.ErrNoRows {
		daylight = nil
	} else if err != nil {
		return err
	}
	ids, err := s.PlantIDs(ctx, tx, lamp.ID)
	if err != nil {
		return err
	}
	if daylight != nil && len(ids) > 0 {
		natural := daylight.SunshineHours
		sunrise, sunset := daylight.Sunrise, daylight.Sunset
		if sunrise != nil && !anyStartedBefore(autos, kept, *sunrise) {
			need, err := s.worstDeficit(ctx, tx, ids, natural, now)
			if err != nil {
				return err
			}
			if err := s.addAuto(ctx, tx, lamp, summary.PlanMorning(need, sunrise, lamp.MorningNotBefore, day, s.Zone), now); err != nil {
				return err
			}
		}
		if sunset != nil && !anyStartedAtOrAfter(autos, kept, *sunset) {
			remaining, err := s.worstDeficit(ctx, tx, ids, natural, now)
			if err != nil {
				return err
			}
			if err := s.addAuto(ctx, tx, lamp, summary.PlanEvening(remaining, sunset, lamp.EveningNotAfter, day, s.Zone), now); err != nil {
				return err
			}
		}
		if sunrise != nil && sunset != nil && !anyDayStarted(autos, kept, *sunrise, *sunset) {
			remaining, err := s.worstDeficit(ctx, tx, ids, natural, now)
			if err != nil {
				return err
			}
			if err := s.addAuto(ctx, tx, lamp, summary.PlanDay(remaining, sunrise, sunset), now); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func anyStartedBefore(autos []*models.LampSession, kept map[int]bool, t time.Time) bool {
	for _, a := range autos {
		if kept[a.ID] && a.StartedAt.Before(t) {
			return true
		}
	}
	return false
}

func anyStartedAtOrAfter(autos []*models.LampSession, kept map[int]bool, t time.Time) bool {
	for _, a := range autos {
		if kept[a.ID] && !a.StartedAt.Before(t) {
			return true
		}
	}
	return false
}

func anyDayStarted(autos []*models.LampSession, kept map[int]bool, sunrise, sunset time.Time) bool {
	for _, a := range autos {
		if kept[a.ID] && !a.StartedAt.Before(sunrise) && a.StartedAt.Before(sunset) {
			return true
		}
	}
	return false
}

// StopAuto — режим больше не «Авто».
func (s *Service) StopAuto(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE lamps SET paused_until=NULL WHERE id=$1`, lamp.ID); err != nil {
		return err
	}
	autos, err := s.autoToday(ctx, tx, lamp, now)
	if err != nil {
		return err
	}
	for _, a := range autos {
		if a.StartedAt.After(now) {
			if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE id=$1`, a.ID); err != nil {
				return err
			}
		} else if a.EndedAt == nil || a.EndedAt.After(now) {
			if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET ended_at=$1 WHERE id=$2`, now, a.ID); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// ---------- розетка ----------

func (s *Service) YandexToken(ctx context.Context, userID int) (*string, error) {
	var enc *string
	err := s.Pool.QueryRow(ctx, `SELECT yandex_token FROM user_settings WHERE user_id=$1`, userID).Scan(&enc)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, nil
	}
	dec, err := secretbox.Decrypt(*enc, s.Secret)
	if err != nil {
		return nil, nil
	}
	return &dec, nil
}

func (s *Service) fail(lamp *models.Lamp, msg string, now time.Time) {
	lamp.LastError = &msg
	lamp.LastErrorAt = &now
}

// SyncPlug — довести розетку до нужного состояния. Команда — только при смене (last_state).
func (s *Service) SyncPlug(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	if lamp.DeviceID == nil {
		return nil
	}
	cover, err := s.coveringSession(ctx, lamp.ID, now)
	if err != nil {
		return err
	}
	want := lamp.ArchivedAt == nil && cover != nil
	if lamp.LastState != nil && *lamp.LastState == want {
		return nil
	}
	var invalid bool
	err = s.Pool.QueryRow(ctx, `SELECT yandex_token_invalid FROM user_settings WHERE user_id=$1`, lamp.UserID).Scan(&invalid)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	token, err := s.YandexToken(ctx, lamp.UserID)
	if err != nil {
		return err
	}
	if token == nil {
		s.fail(lamp, "Не задан токен Яндекса", now)
	} else if invalid {
		s.fail(lamp, "Токен Яндекса недействителен — обновите его в настройках", now)
	} else {
		err := s.yandexSetOn(*token, *lamp.DeviceID, want)
		if err != nil {
			var authErr *yandex.YandexAuthError
			if errors.As(err, &authErr) {
				if _, err := s.Pool.Exec(ctx, `UPDATE user_settings SET yandex_token_invalid=true WHERE user_id=$1`, lamp.UserID); err != nil {
					return err
				}
				s.fail(lamp, err.Error(), now)
			} else {
				s.fail(lamp, err.Error(), now)
			}
		} else {
			lamp.LastState = &want
			lamp.LastError = nil
			lamp.LastErrorAt = nil
		}
	}
	_, err = s.Pool.Exec(ctx, `UPDATE lamps SET last_state=$1, last_error=$2, last_error_at=$3 WHERE id=$4`,
		lamp.LastState, lamp.LastError, lamp.LastErrorAt, lamp.ID)
	return err
}

// ---------- изменения лампы ----------

// Update — PATCH лампы (поля уже проверены). Смена режима убирает хвосты старого режима.
func (s *Service) Update(ctx context.Context, lamp *models.Lamp, data map[string]any, ids []int, hasIDs bool, now time.Time) error {
	oldMode, oldDevice := lamp.Mode, lamp.DeviceID
	for k, v := range data {
		if err := applyLamp(lamp, k, v, s.Zone); err != nil {
			return err
		}
	}
	if lamp.DeviceID != oldDevice {
		lamp.LastState, lamp.LastError, lamp.LastErrorAt = nil, nil, nil
	}
	if err := s.persistLamp(ctx, lamp); err != nil {
		return err
	}
	if hasIDs {
		if err := s.SetPlants(ctx, lamp, ids, now); err != nil {
			return err
		}
	}
	if oldMode == "auto" && lamp.Mode != "auto" {
		if err := s.StopAuto(ctx, lamp, now); err != nil {
			return err
		}
	}
	if oldMode == "schedule" && lamp.Mode != "schedule" {
		if _, err := s.replaceSchedule(ctx, lamp.UserID, lamp.ID, nil); err != nil {
			return err
		}
	}
	return s.AfterChange(ctx, lamp, now)
}

func (s *Service) persistLamp(ctx context.Context, lamp *models.Lamp) error {
	_, err := s.Pool.Exec(ctx, `UPDATE lamps SET name=$1, mode=$2::lamp_mode, device_id=$3, device_name=$4,
		morning_not_before=$5, evening_not_after=$6, last_state=$7, last_error=$8, last_error_at=$9, paused_until=$10
		WHERE id=$11`,
		lamp.Name, lamp.Mode, lamp.DeviceID, lamp.DeviceName, lamp.MorningNotBefore, lamp.EveningNotAfter,
		lamp.LastState, lamp.LastError, lamp.LastErrorAt, lamp.PausedUntil, lamp.ID)
	return err
}

func applyLamp(lamp *models.Lamp, key string, v any, zone *time.Location) error {
	switch key {
	case "name":
		lamp.Name = v.(string)
	case "mode":
		lamp.Mode = v.(string)
	case "device_id":
		lamp.DeviceID = toPtrStr(v)
	case "device_name":
		lamp.DeviceName = toPtrStr(v)
	case "morning_not_before":
		lamp.MorningNotBefore = v.(time.Time)
	case "evening_not_after":
		lamp.EveningNotAfter = v.(time.Time)
	}
	_ = zone
	return nil
}

func toPtrStr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}

func (s *Service) AfterChange(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	if lamp.Mode == "auto" {
		if err := s.ReplanAuto(ctx, lamp, now); err != nil {
			return err
		}
	}
	return s.SyncPlug(ctx, lamp, now)
}

func (s *Service) replaceSchedule(ctx context.Context, userID, lampID int, intervals []summary.Interval) ([]models.LampSchedule, error) {
	return light.ReplaceSchedule(ctx, s.Pool, userID, lampID, intervals, s.Zone)
}

// MovePlant — перенести растение; досветка старой и новой лампы пересчитывается сразу.
func (s *Service) MovePlant(ctx context.Context, plantID int, lampID *int, now time.Time) error {
	old, err := s.CurrentLampID(ctx, plantID)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := assign(ctx, tx, plantID, lampID, now); err != nil {
		tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, lid := range []*int{old, lampID} {
		if lid != nil {
			lamp, err := s.GetLamp(ctx, *lid)
			if err != nil {
				return err
			}
			if lamp != nil && lamp.ArchivedAt == nil {
				if err := s.AfterChange(ctx, lamp, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Service) GetLamp(ctx context.Context, id int) (*models.Lamp, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, name, mode::text, device_id, device_name,
		morning_not_before, evening_not_after, last_state, last_error, last_error_at, archived_at, paused_until
		FROM lamps WHERE id=$1`, id)
	return scanLamp(row)
}

func scanLamp(row pgx.Row) (*models.Lamp, error) {
	var l models.Lamp
	var mode string
	err := row.Scan(&l.ID, &l.UserID, &l.Name, &mode, &l.DeviceID, &l.DeviceName,
		&l.MorningNotBefore, &l.EveningNotAfter, &l.LastState, &l.LastError, &l.LastErrorAt, &l.ArchivedAt, &l.PausedUntil)
	if err != nil {
		return nil, err
	}
	l.Mode = mode
	return &l, nil
}

// Archive — «удалить» лампу: растения без лампы, горящее гаснет, будущее и расписание удаляются.
func (s *Service) Archive(ctx context.Context, lamp *models.Lamp, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE lamps SET archived_at=$1, paused_until=NULL WHERE id=$2`, now, lamp.ID); err != nil {
		return err
	}
	lamp.ArchivedAt = &now
	lamp.PausedUntil = nil
	ids, err := s.PlantIDs(ctx, tx, lamp.ID)
	if err != nil {
		return err
	}
	for _, pid := range ids {
		if err := assign(ctx, tx, pid, nil, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE lamp_sessions SET ended_at=$1 WHERE lamp_id=$2 AND (ended_at IS NULL OR ended_at > $1) AND started_at <= $1`, now, lamp.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM lamp_sessions WHERE lamp_id=$1 AND started_at > $2`, lamp.ID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM lamp_schedules WHERE lamp_id=$1`, lamp.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.SyncPlug(ctx, lamp, now)
}

// ---------- ответы API ----------

func (s *Service) ActivePause(lamp *models.Lamp, now time.Time) *time.Time {
	if lamp.PausedUntil != nil && lamp.PausedUntil.After(now) {
		return lamp.PausedUntil
	}
	return nil
}

func (s *Service) planned(ctx context.Context, lamp *models.Lamp, now time.Time) ([]*models.LampSession, error) {
	return s.autoToday(ctx, s.Pool, lamp, now)
}

// Tick — сессии по расписаниям на сегодня, досветка и розетки — для всех активных учёток.
func (s *Service) Tick(ctx context.Context, now time.Time) error {
	day := summary.LocalDate(now, s.Zone)
	rows, err := s.Pool.Query(ctx, `SELECT id FROM users WHERE blocked_at IS NULL`)
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
	for _, uid := range ids {
		_, _ = light.EnsureScheduleSessions(ctx, s.Pool, uid, day, s.Zone)
	}
	lamps, err := s.LampsToTick(ctx)
	if err != nil {
		return err
	}
	for _, lamp := range lamps {
		expired := lamp.PausedUntil != nil && !lamp.PausedUntil.After(now)
		if expired {
			lamp.PausedUntil = nil
			if _, err := s.Pool.Exec(ctx, `UPDATE lamps SET paused_until=NULL WHERE id=$1`, lamp.ID); err != nil {
				return err
			}
		}
		if lamp.Mode == "auto" {
			if err := s.ReplanAuto(ctx, lamp, now); err != nil {
				continue
			}
		}
		_ = s.SyncPlug(ctx, lamp, now)
	}
	return nil
}

func (s *Service) LampsToTick(ctx context.Context) ([]*models.Lamp, error) {
	rows, err := s.Pool.Query(ctx, `SELECT l.id, l.user_id, l.name, l.mode::text, l.device_id, l.device_name,
		l.morning_not_before, l.evening_not_after, l.last_state, l.last_error, l.last_error_at, l.archived_at, l.paused_until
		FROM lamps l JOIN users u ON u.id=l.user_id WHERE l.archived_at IS NULL AND u.blocked_at IS NULL ORDER BY l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Lamp
	for rows.Next() {
		l, err := scanLamp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func intPtr(n int) *int { return &n }

func intsToAny(ids []int) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func stringPtr(s string) *string { return &s }

// CoveringSession — сессия лампы, горящая прямо сейчас (обёртка для сводки).
func (s *Service) CoveringSession(ctx context.Context, lampID int, now time.Time) (*models.LampSession, error) {
	return s.coveringSession(ctx, lampID, now)
}

// CoveringSessions — все горящие сейчас сессии лампы (ручная + досветка могут идти одновременно).
func (s *Service) CoveringSessions(ctx context.Context, lampID int, now time.Time) ([]*models.LampSession, error) {
	return s.coveringSessions(ctx, lampID, now)
}

// Brief — лампа растения в сводке.
func (s *Service) Brief(ctx context.Context, lamp *models.Lamp, now time.Time) (*schema.LampBrief, error) {
	if lamp == nil {
		return nil, nil
	}
	cover, err := s.coveringSession(ctx, lamp.ID, now)
	if err != nil {
		return nil, err
	}
	planned, err := s.planned(ctx, lamp, now)
	if err != nil {
		return nil, err
	}
	pl := make([]schema.PlannedInterval, 0, len(planned))
	for _, p := range planned {
		pl = append(pl, schema.PlannedInterval{Start: render.Time(p.StartedAt, s.Zone), End: optTime(p.EndedAt, s.Zone)})
	}
	pause := s.ActivePause(lamp, now)
	return &schema.LampBrief{
		ID:          lamp.ID,
		Name:        lamp.Name,
		Mode:        lamp.Mode,
		IsOn:        cover != nil,
		HasDevice:   lamp.DeviceID != nil,
		PausedUntil: optTimePtr(pause, s.Zone),
		Planned:     pl,
		LastError:   lamp.LastError,
	}, nil
}

// LampOut — полный ответ лампы.
func (s *Service) LampOut(ctx context.Context, lamp *models.Lamp, now time.Time) (*schema.LampOut, error) {
	ids, err := s.PlantIDs(ctx, s.Pool, lamp.ID)
	if err != nil {
		return nil, err
	}
	cover, err := s.coveringSession(ctx, lamp.ID, now)
	if err != nil {
		return nil, err
	}
	schRows, err := light.SchedulesFor(ctx, s.Pool, lamp.ID)
	if err != nil {
		return nil, err
	}
	sch := make([]schema.ScheduleInterval, 0, len(schRows))
	for _, r := range schRows {
		sch = append(sch, schema.ScheduleInterval{StartTime: render.ClockTime(r.StartTime), EndTime: render.ClockTime(r.EndTime)})
	}
	planned, err := s.planned(ctx, lamp, now)
	if err != nil {
		return nil, err
	}
	pl := make([]schema.PlannedInterval, 0, len(planned))
	for _, p := range planned {
		pl = append(pl, schema.PlannedInterval{Start: render.Time(p.StartedAt, s.Zone), End: optTime(p.EndedAt, s.Zone)})
	}
	pause := s.ActivePause(lamp, now)
	return &schema.LampOut{
		LampFields: schema.LampFields{
			Name:             lamp.Name,
			Mode:             lamp.Mode,
			DeviceID:         lamp.DeviceID,
			DeviceName:       lamp.DeviceName,
			MorningNotBefore: render.ClockTime(lamp.MorningNotBefore),
			EveningNotAfter:  render.ClockTime(lamp.EveningNotAfter),
		},
		ID:          lamp.ID,
		LastState:   lamp.LastState,
		LastError:   lamp.LastError,
		LastErrorAt: optTimePtr(lamp.LastErrorAt, s.Zone),
		PausedUntil: optTimePtr(pause, s.Zone),
		PlantIDs:    ids,
		IsOn:        cover != nil,
		Schedule:    sch,
		Planned:     pl,
	}, nil
}

func optTime(t *time.Time, loc *time.Location) *string {
	if t == nil {
		return nil
	}
	s := render.Time(*t, loc)
	return &s
}

func optTimePtr(t *time.Time, loc *time.Location) *string {
	return optTime(t, loc)
}

// ListDevices — безопасная обёртка: подменяемая в тестах, в сеть без подмены не ходит.
func (s *Service) ListDevices(token string) ([]yandex.Device, error) {
	if s.YandexListDevices != nil {
		return s.YandexListDevices(token)
	}
	return yandex.ListDevices(token)
}

// SetOn — безопасная обёртка команды розетки.
func (s *Service) SetOn(token, deviceID string, on bool) error {
	if s.YandexSetOn != nil {
		return s.YandexSetOn(token, deviceID, on)
	}
	return yandex.SetOn(token, deviceID, on)
}
