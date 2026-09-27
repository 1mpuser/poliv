package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"poliv/internal/models"
	"poliv/internal/plants"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/summary"
	"poliv/internal/users"
)

func (s *Server) ownedPlant(ctx context.Context, plantID, userID int) (*models.Plant, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, name, species, location, pot_size_l, added_at, notes,
		water_interval_days, fertilizing_enabled, light_target_hours, repot_check_interval_months
		FROM plants WHERE id=$1 AND user_id=$2`, plantID, userID)
	p, err := plants.ScanPlant(row)
	if err == pgx.ErrNoRows {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

var errNotFound = &notFoundErr{}

type notFoundErr struct{}

func (e *notFoundErr) Error() string { return "Запись не найдена" }

func (s *Server) listPlants(w http.ResponseWriter, r *http.Request, user *models.User) {
	plants, err := s.Plants.PlantsByUser(r.Context(), user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]schema.PlantOut, 0, len(plants))
	for _, p := range plants {
		out = append(out, *plantToSchema(&p, s.Zone))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getPlant(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	p, err := s.ownedPlant(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	writeJSON(w, http.StatusOK, plantToSchema(p, s.Zone))
}

func (s *Server) createPlant(w http.ResponseWriter, r *http.Request, user *models.User) {
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	name := mustString(b, "name")
	if strings.TrimSpace(name) == "" {
		writeValidation(w, nil)
		return
	}
	species := strOr(b, "species", "")
	location := bStr(b, "location")
	potSize := bFloat(b, "pot_size_l")
	notes := bStr(b, "notes")
	water := intOr(b, "water_interval_days", 4)
	fert := boolOr(b, "fertilizing_enabled", true)
	lightH := floatOr(b, "light_target_hours", 12)
	repotM := intOr(b, "repot_check_interval_months", 12)
	_ = summary.LocalDate
	var id int
	err = s.Pool.QueryRow(r.Context(), `INSERT INTO plants
		(user_id, name, species, location, pot_size_l, notes, water_interval_days, fertilizing_enabled, light_target_hours, repot_check_interval_months)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		user.ID, name, species, location, potSize, notes, water, fert, lightH, repotM).Scan(&id)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := s.ownedPlant(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, plantToSchema(p, s.Zone))
}

func (s *Server) updatePlant(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	set := map[string]any{}
	if v := bStr(b, "name"); b.has("name") {
		set["name"] = *v
	}
	if b.has("species") {
		set["species"] = bStr(b, "species")
	}
	if b.has("location") {
		set["location"] = bStr(b, "location")
	}
	if b.has("pot_size_l") {
		set["pot_size_l"] = bFloat(b, "pot_size_l")
	}
	if b.has("notes") {
		set["notes"] = bStr(b, "notes")
	}
	if b.has("water_interval_days") {
		set["water_interval_days"] = intOr(b, "water_interval_days", 0)
	}
	if b.has("fertilizing_enabled") {
		set["fertilizing_enabled"] = boolOr(b, "fertilizing_enabled", false)
	}
	if b.has("light_target_hours") {
		set["light_target_hours"] = floatOr(b, "light_target_hours", 0)
	}
	if b.has("repot_check_interval_months") {
		set["repot_check_interval_months"] = intOr(b, "repot_check_interval_months", 0)
	}
	if err := applySet(r.Context(), s.Pool, "plants", id, set); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := s.ownedPlant(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plantToSchema(p, s.Zone))
}

func (s *Server) deletePlant(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM plants WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) allSummaries(w http.ResponseWriter, r *http.Request, user *models.User) {
	ids, err := s.Plants.PlantsByUser(r.Context(), user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings, err := getUserSettingsFor(r.Context(), s, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	ferts, err := s.Plants.FertilizersForUser(r.Context(), user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	daylight, err := s.currentDaylight(r.Context(), user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]schema.PlantSummary, 0, len(ids))
	summaryFerts := plants.ToSummaryFerts(ferts)
	for i := range ids {
		ss, err := s.Plants.BuildOne(r.Context(), &ids[i], settings, summaryFerts, now, daylight)
		if err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, *ss)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) plantSummary(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	ss, err := s.Plants.BuildSummary(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

func (s *Server) plantHistory(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	validTypes := map[string]bool{"water": true, "check": true, "feed": true, "lamp": true, "repot": true}
	types := map[string]bool{}
	for _, t := range r.URL.Query()["types"] {
		// как Python-референс: неизвестный тип события — 422, а не молчаливо пустой ответ
		if !validTypes[t] {
			writeValidation(w, []any{"неверный тип события"})
			return
		}
		types[t] = true
	}
	if len(types) == 0 {
		for _, t := range []string{"water", "check", "feed", "lamp", "repot"} {
			types[t] = true
		}
	}
	var dateFrom, dateTo *time.Time
	if v := r.URL.Query().Get("date_from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			writeValidation(w, nil)
			return
		}
		d := summary.LocalMidnight(summary.Date(t), s.Zone)
		dateFrom = &d
	}
	if v := r.URL.Query().Get("date_to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			writeValidation(w, nil)
			return
		}
		d := summary.LocalMidnight(summary.DateAddDays(summary.Date(t), 1), s.Zone)
		dateTo = &d
	}
	events, err := s.Plants.History(r.Context(), id, types, dateFrom, dateTo, s.Zone)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) plantWeekly(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	weeks := 8
	if v := r.URL.Query().Get("weeks"); v != "" {
		n, err := parseInt(v)
		if err != nil {
			writeValidation(w, nil)
			return
		}
		weeks = n
	}
	out, err := s.Plants.Weekly(r.Context(), user.ID, id, weeks)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setPlantLamp(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	var lampID *int
	if raw, ok := b["lamp_id"]; ok && string(raw) != "null" {
		n, err := parseInt(string(raw))
		if err != nil {
			writeValidation(w, nil)
			return
		}
		lampID = &n
		// лампа должна быть своя
		if _, err := s.ownedLamp(r.Context(), *lampID, user.ID); err != nil {
			writeDetail(w, http.StatusNotFound, "Лампа не найдена")
			return
		}
	}
	now := time.Now().UTC()
	if err := s.Lamps.MovePlant(r.Context(), id, lampID, now); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var outLampID *int
	_ = outLampID
	writeJSON(w, http.StatusOK, map[string]any{"lamp_id": lampID})
}

// ---------- вспомогательные конструкции ----------

func plantToSchema(p *models.Plant, zone *time.Location) *schema.PlantOut {
	return &schema.PlantOut{
		PlantFields: schema.PlantFields{
			Name: p.Name, Species: p.Species, Location: p.Location,
			PotSizeL: render.MarshalF(p.PotSizeL), Notes: p.Notes,
			WaterIntervalDays: p.WaterIntervalDays, FertilizingEnabled: p.FertilizingEnabled,
			LightTargetHours: render.F(p.LightTargetHours), RepotCheckMonths: p.RepotCheckIntervalMonths,
		},
		ID: p.ID, AddedAt: render.Time(p.AddedAt, zone),
	}
}

func getUserSettingsFor(ctx context.Context, s *Server, userID int) (*models.UserSettings, error) {
	return users.GetUserSettings(ctx, s.Pool, userID)
}

func (s *Server) currentDaylight(ctx context.Context, userID int) (*models.DaylightDay, error) {
	return s.Plants.CurrentDaylight(ctx, userID)
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func mustString(b body, key string) string {
	v := bStr(b, key)
	if v == nil {
		return ""
	}
	return *v
}

func bStr(b body, key string) *string {
	v, _ := b.string(key)
	return v
}

func strOr(b body, key, def string) string {
	v := bStr(b, key)
	if v == nil {
		return def
	}
	return *v
}

func bFloat(b body, key string) *float64 {
	v, _ := b.float(key)
	return v
}

func floatOr(b body, key string, def float64) float64 {
	v := b.floatFloat(key)
	if v == nil {
		return def
	}
	return *v
}

func (b body) floatFloat(key string) *float64 {
	v, _ := b.float(key)
	return v
}

func intOr(b body, key string, def int) int {
	v, _ := b.int(key)
	if v == nil {
		return def
	}
	return *v
}

func boolOr(b body, key string, def bool) bool {
	v, _ := b.boolean(key)
	if v == nil {
		return def
	}
	return *v
}
