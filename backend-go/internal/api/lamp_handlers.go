package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
)

func (s *Server) ownedLampSession(ctx context.Context, id, userID int) (*models.LampSession, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, lamp_id, source::text, started_at, ended_at,
		planned_hours_per_day, schedule_id, after_pause FROM lamp_sessions WHERE id=$1 AND user_id=$2`, id, userID)
	var sess models.LampSession
	var source string
	if err := row.Scan(&sess.ID, &sess.UserID, &sess.LampID, &source, &sess.StartedAt, &sess.EndedAt,
		&sess.PlannedHours, &sess.ScheduleID, &sess.AfterPause); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound
		}
		return nil, err
	}
	sess.Source = source
	return &sess, nil
}

func sessOut(sess *models.LampSession, loc *time.Location) schema.LampSessionOut {
	return schema.LampSessionOut{
		ID: sess.ID, LampID: sess.LampID, Source: sess.Source,
		StartedAt: render.Time(sess.StartedAt, loc), EndedAt: optTime(sess.EndedAt, loc),
		PlannedHours: render.F(sess.PlannedHours),
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, user *models.User) {
	q := `SELECT id, user_id, lamp_id, source::text, started_at, ended_at, planned_hours_per_day, schedule_id, after_pause
		FROM lamp_sessions WHERE user_id=$1`
	args := []any{user.ID}
	if lampID := r.URL.Query().Get("lamp_id"); lampID != "" {
		q += " AND lamp_id=$2"
		n, _ := parseInt(lampID)
		args = append(args, n)
	}
	if open := r.URL.Query().Get("open"); open != "" {
		if open == "true" {
			q += " AND ended_at IS NULL"
		} else if open == "false" {
			q += " AND ended_at IS NOT NULL"
		}
	}
	q += " ORDER BY started_at DESC"
	rows, err := s.Pool.Query(r.Context(), q, args...)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []schema.LampSessionOut{}
	for rows.Next() {
		var sess models.LampSession
		var source string
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.LampID, &source, &sess.StartedAt, &sess.EndedAt,
			&sess.PlannedHours, &sess.ScheduleID, &sess.AfterPause); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		sess.Source = source
		out = append(out, sessOut(&sess, s.Zone))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, user *models.User) {
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	lampID := intOr(b, "lamp_id", 0)
	lamp, err := s.ownedLamp(r.Context(), lampID, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	started, err := b.timeField("started_at")
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if started == nil {
		t := time.Now().UTC()
		started = &t
	}
	ended, err := b.timeField("ended_at")
	if err != nil {
		writeValidation(w, nil)
		return
	}
	planned := floatOr(b, "planned_hours_per_day", 12)
	var id int
	err = s.Pool.QueryRow(r.Context(), `INSERT INTO lamp_sessions (user_id, lamp_id, source, started_at, ended_at, planned_hours_per_day)
		VALUES ($1,$2,'manual',$3,$4,$5) RETURNING id`, user.ID, lamp.ID, *started, ended, planned).Scan(&id)
	if err != nil {
		if isIntegrity(err) {
			writeDetail(w, http.StatusConflict, "Лампа уже горит, или время выключения раньше включения")
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	fresh, _ := s.ownedLampSession(r.Context(), id, user.ID)
	_ = s.Lamps.SyncPlug(r.Context(), lamp, time.Now().UTC())
	writeJSON(w, http.StatusCreated, sessOut(fresh, s.Zone))
}

func (s *Server) toggleSession(w http.ResponseWriter, r *http.Request, user *models.User) {
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	plantID := intOr(b, "plant_id", 0)
	if _, err := s.ownedPlant(r.Context(), plantID, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	lampID, err := s.Lamps.CurrentLampID(r.Context(), plantID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if lampID == nil {
		writeDetail(w, http.StatusBadRequest, "У растения нет лампы — привяжите её в настройках")
		return
	}
	lamp, err := s.ownedLamp(r.Context(), *lampID, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Лампа не найдена")
		return
	}
	s.toggleOut(w, r, lamp)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	sess, err := s.ownedLampSession(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Сессия лампы не найдена")
		return
	}
	writeJSON(w, http.StatusOK, sessOut(sess, s.Zone))
}

func (s *Server) updateSession(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	sess, err := s.ownedLampSession(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Сессия лампы не найдена")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	set := map[string]any{}
	if b.has("started_at") {
		t, err := b.timeField("started_at")
		if err != nil {
			writeValidation(w, nil)
			return
		}
		set["started_at"] = t
	}
	if b.has("ended_at") {
		t, err := b.timeField("ended_at")
		if err != nil {
			writeValidation(w, nil)
			return
		}
		set["ended_at"] = t
	}
	if b.has("planned_hours_per_day") {
		set["planned_hours_per_day"] = floatOr(b, "planned_hours_per_day", 12)
	}
	if err := applySet(r.Context(), s.Pool, "lamp_sessions", id, set); err != nil {
		if isIntegrity(err) {
			writeDetail(w, http.StatusConflict, "Лампа уже горит, или время выключения раньше включения")
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lamp, err := s.ownedLamp(r.Context(), sess.LampID, user.ID)
	if err == nil {
		_ = s.Lamps.SyncPlug(r.Context(), lamp, time.Now().UTC())
	}
	fresh, _ := s.ownedLampSession(r.Context(), id, user.ID)
	writeJSON(w, http.StatusOK, sessOut(fresh, s.Zone))
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	sess, err := s.ownedLampSession(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Сессия лампы не найдена")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM lamp_sessions WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	lamp, err := s.ownedLamp(r.Context(), sess.LampID, user.ID)
	if err == nil {
		_ = s.Lamps.SyncPlug(r.Context(), lamp, time.Now().UTC())
	}
	writeJSON(w, http.StatusNoContent, nil)
}
