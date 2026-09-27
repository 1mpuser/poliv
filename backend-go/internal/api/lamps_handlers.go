package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"poliv/internal/lamps"
	"poliv/internal/light"
	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/summary"
	"poliv/internal/users"
)

func (s *Server) ownedLamp(ctx context.Context, id, userID int) (*models.Lamp, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, name, mode::text, device_id, device_name,
		morning_not_before, evening_not_after, last_state, last_error, last_error_at, archived_at, paused_until
		FROM lamps WHERE id=$1 AND user_id=$2 AND archived_at IS NULL`, id, userID)
	var l models.Lamp
	var mode string
	if err := row.Scan(&l.ID, &l.UserID, &l.Name, &mode, &l.DeviceID, &l.DeviceName,
		&l.MorningNotBefore, &l.EveningNotAfter, &l.LastState, &l.LastError, &l.LastErrorAt, &l.ArchivedAt, &l.PausedUntil); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound
		}
		return nil, err
	}
	l.Mode = mode
	return &l, nil
}

func (s *Server) listLamps(w http.ResponseWriter, r *http.Request, user *models.User) {
	rows, err := s.Pool.Query(r.Context(), `SELECT id, user_id, name, mode::text, device_id, device_name,
		morning_not_before, evening_not_after, last_state, last_error, last_error_at, archived_at, paused_until
		FROM lamps WHERE user_id=$1 AND archived_at IS NULL ORDER BY id`, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	now := time.Now().UTC()
	out := []schema.LampOut{}
	for rows.Next() {
		var l models.Lamp
		var mode string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Name, &mode, &l.DeviceID, &l.DeviceName,
			&l.MorningNotBefore, &l.EveningNotAfter, &l.LastState, &l.LastError, &l.LastErrorAt, &l.ArchivedAt, &l.PausedUntil); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		l.Mode = mode
		lo, err := s.Lamps.LampOut(r.Context(), &l, now)
		if err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, *lo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	l, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	lo, err := s.Lamps.LampOut(r.Context(), l, time.Now().UTC())
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lo)
}

func (s *Server) createLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	name := strOr(b, "name", "")
	if name == "" {
		writeValidation(w, nil)
		return
	}
	mode := strOr(b, "mode", "manual")
	morning, err := b.clockField("morning_not_before", s.Zone)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if morning.IsZero() {
		morning = clockAt(s.Zone, 6, 0)
	}
	evening, err := b.clockField("evening_not_after", s.Zone)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if evening.IsZero() {
		evening = clockAt(s.Zone, 23, 0)
	}
	if !evening.After(morning) {
		writeDetail(w, http.StatusBadRequest, "«Вечером не позже» должно быть позже «утром не раньше»")
		return
	}
	if mode == "auto" {
		sett, err := users.GetUserSettings(r.Context(), s.Pool, user.ID)
		if err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if sett.Latitude == nil {
			writeDetail(w, http.StatusBadRequest, "Для режима «Авто» задайте город в настройках (раздел «Свет»)")
			return
		}
	}
	ids, err := b.intList("plant_ids")
	if err != nil {
		writeValidation(w, nil)
		return
	}
	for _, pid := range ids {
		if _, err := s.ownedPlant(r.Context(), pid, user.ID); err != nil {
			writeDetail(w, http.StatusNotFound, "Растение не найдено")
			return
		}
	}
	lamp := models.Lamp{
		UserID: user.ID, Name: name, Mode: mode,
		DeviceID: bStr(b, "device_id"), DeviceName: bStr(b, "device_name"),
		MorningNotBefore: morning, EveningNotAfter: evening,
	}
	var id int
	if err := s.Pool.QueryRow(r.Context(), `INSERT INTO lamps
		(user_id, name, mode, device_id, device_name, morning_not_before, evening_not_after)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		user.ID, name, mode, lamp.DeviceID, lamp.DeviceName, morning, evening).Scan(&id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lamp.ID = id
	if err := s.Lamps.SetPlants(r.Context(), &lamp, ids, time.Now().UTC()); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Lamps.AfterChange(r.Context(), &lamp, time.Now().UTC()); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lo, err := s.Lamps.LampOut(r.Context(), &lamp, time.Now().UTC())
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, lo)
}

func (s *Server) updateLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lamp, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	mode := lamp.Mode
	morning := lamp.MorningNotBefore
	evening := lamp.EveningNotAfter
	if b.has("mode") {
		mode = strOr(b, "mode", lamp.Mode)
	}
	if b.has("morning_not_before") {
		morning, err = b.clockField("morning_not_before", s.Zone)
		if err != nil {
			writeValidation(w, nil)
			return
		}
	}
	if b.has("evening_not_after") {
		evening, err = b.clockField("evening_not_after", s.Zone)
		if err != nil {
			writeValidation(w, nil)
			return
		}
	}
	if !evening.After(morning) {
		writeDetail(w, http.StatusBadRequest, "«Вечером не позже» должно быть позже «утром не раньше»")
		return
	}
	if mode == "auto" {
		sett, err := users.GetUserSettings(r.Context(), s.Pool, user.ID)
		if err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if sett.Latitude == nil {
			writeDetail(w, http.StatusBadRequest, "Для режима «Авто» задайте город в настройках (раздел «Свет»)")
			return
		}
	}
	data := map[string]any{}
	if b.has("name") {
		data["name"] = strOr(b, "name", lamp.Name)
	}
	if b.has("mode") {
		data["mode"] = mode
	}
	if b.has("device_id") {
		data["device_id"] = bStr(b, "device_id")
	}
	if b.has("device_name") {
		data["device_name"] = bStr(b, "device_name")
	}
	if b.has("morning_not_before") {
		data["morning_not_before"] = morning
	}
	if b.has("evening_not_after") {
		data["evening_not_after"] = evening
	}
	var ids []int
	hasIDs := b.has("plant_ids")
	if hasIDs {
		ids, err = b.intList("plant_ids")
		if err != nil {
			writeValidation(w, nil)
			return
		}
		for _, pid := range ids {
			if _, err := s.ownedPlant(r.Context(), pid, user.ID); err != nil {
				writeDetail(w, http.StatusNotFound, "Растение не найдено")
				return
			}
		}
	}
	now := time.Now().UTC()
	if err := s.Lamps.Update(r.Context(), lamp, data, ids, hasIDs, now); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lo, err := s.Lamps.LampOut(r.Context(), lamp, now)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lo)
}

func (s *Server) archiveLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lamp, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	if err := s.Lamps.Archive(r.Context(), lamp, time.Now().UTC()); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type scheduleSet struct {
	Intervals []intervalIn `json:"intervals"`
}

type intervalIn struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

func (s *Server) setLampSchedule(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lamp, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	var b scheduleSet
	if err := decodeJSON(r, &b); err != nil {
		writeValidation(w, nil)
		return
	}
	ivs := make([]summary.Interval, 0, len(b.Intervals))
	for _, iv := range b.Intervals {
		start, err := parseClock(iv.StartTime, s.Zone)
		if err != nil {
			writeValidation(w, nil)
			return
		}
		end, err := parseClock(iv.EndTime, s.Zone)
		if err != nil {
			writeValidation(w, nil)
			return
		}
		ivs = append(ivs, summary.Interval{Start: start, End: end})
	}
	valid, err := light.ValidateIntervals(ivs)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	if lamp.Mode != "schedule" && len(valid) > 0 {
		writeDetail(w, http.StatusBadRequest, "Расписание задаётся в режиме «По расписанию»")
		return
	}
	rows, err := light.ReplaceSchedule(r.Context(), s.Pool, user.ID, lamp.ID, valid, s.Zone)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Lamps.SyncPlug(r.Context(), lamp, time.Now().UTC())
	out := make([]schema.ScheduleInterval, 0, len(rows))
	for _, rw := range rows {
		out = append(out, schema.ScheduleInterval{StartTime: render.ClockTime(rw.StartTime), EndTime: render.ClockTime(rw.EndTime)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) toggleLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lamp, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	s.toggleOut(w, r, lamp)
}

func (s *Server) pauseLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lamp, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	if err := s.Lamps.Pause(r.Context(), lamp, time.Now().UTC()); err != nil {
		if err == lamps.ErrLampNotOn {
			writeDetail(w, http.StatusConflict, "Лампа не горит — её не нужно ставить на паузу")
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lo, err := s.Lamps.LampOut(r.Context(), lamp, time.Now().UTC())
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lo)
}

func (s *Server) resumeLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lamp, err := s.ownedLamp(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	if err := s.Lamps.Resume(r.Context(), lamp, time.Now().UTC()); err != nil {
		if err == lamps.ErrLampNotPaused {
			writeDetail(w, http.StatusConflict, "Лампа не на паузе")
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lo, err := s.Lamps.LampOut(r.Context(), lamp, time.Now().UTC())
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lo)
}

func clockAt(loc *time.Location, h, m int) time.Time {
	return time.Date(2000, 1, 1, h, m, 0, 0, loc)
}

func parseClock(s string, loc *time.Location) (time.Time, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return time.Time{}, errNotFound
	}
	h, err := parseInt(parts[0])
	if err != nil {
		return time.Time{}, err
	}
	m, err := parseInt(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	sec := 0
	if len(parts) == 3 {
		sec, _ = parseInt(parts[2])
	}
	return time.Date(2000, 1, 1, h, m, sec, 0, loc), nil
}

// toggleOut — общий ответ кнопки лампы.
func (s *Server) toggleOut(w http.ResponseWriter, r *http.Request, lamp *models.Lamp) {
	res, err := s.Lamps.Toggle(r.Context(), lamp, time.Now().UTC())
	if err != nil {
		writeDetail(w, http.StatusConflict, "Лампа уже горит, или время выключения раньше включения")
		return
	}
	sess := schema.LampSessionOut{
		ID: res.Session.ID, LampID: res.Session.LampID, Source: res.Session.Source,
		StartedAt: render.Time(res.Session.StartedAt, s.Zone), EndedAt: optTime(res.Session.EndedAt, s.Zone),
		PlannedHours: render.F(res.Session.PlannedHours),
	}
	writeJSON(w, http.StatusOK, schema.LampToggleOut{
		IsOn: res.IsOn, Session: sess, PreviousEndedAt: optTime(res.PreviousEndedAt, s.Zone),
		PlugError: lamp.LastError,
	})
}
