package api

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
)

func (s *Server) ownedFertilizer(ctx context.Context, id, userID int) (*models.FertilizerType, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, name, npk, root_dose_ml_per_l, foliar_dose_ml_per_l,
		interval_days_active_season, interval_days_dormant_season FROM fertilizer_types WHERE id=$1 AND user_id=$2`, id, userID)
	var f models.FertilizerType
	if err := row.Scan(&f.ID, &f.UserID, &f.Name, &f.NPK, &f.RootDoseMlPerL, &f.FoliarDoseMlPerL,
		&f.IntervalDaysActiveSeason, &f.IntervalDaysDormantSeason); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound
		}
		return nil, err
	}
	return &f, nil
}

func fertSchema(f *models.FertilizerType) schema.Fertilizer {
	return schema.Fertilizer{
		ID: f.ID, Name: f.Name, NPK: f.NPK,
		RootDoseMlPerL: render.MarshalF(f.RootDoseMlPerL), FoliarDoseMlPerL: render.MarshalF(f.FoliarDoseMlPerL),
		IntervalDaysActive: f.IntervalDaysActiveSeason, IntervalDaysDormant: f.IntervalDaysDormantSeason,
	}
}

func (s *Server) listFertilizers(w http.ResponseWriter, r *http.Request, user *models.User) {
	rows, err := s.Pool.Query(r.Context(), `SELECT id, user_id, name, npk, root_dose_ml_per_l, foliar_dose_ml_per_l,
		interval_days_active_season, interval_days_dormant_season FROM fertilizer_types WHERE user_id=$1 ORDER BY id`, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []schema.Fertilizer{}
	for rows.Next() {
		var f models.FertilizerType
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &f.NPK, &f.RootDoseMlPerL, &f.FoliarDoseMlPerL,
			&f.IntervalDaysActiveSeason, &f.IntervalDaysDormantSeason); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, fertSchema(&f))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createFertilizer(w http.ResponseWriter, r *http.Request, user *models.User) {
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
	npk := strOr(b, "npk", "")
	root := bFloat(b, "root_dose_ml_per_l")
	foliar := bFloat(b, "foliar_dose_ml_per_l")
	active := intOr(b, "interval_days_active_season", 0)
	dormant := bInt(b, "interval_days_dormant_season")
	var id int
	err = s.Pool.QueryRow(r.Context(), `INSERT INTO fertilizer_types
		(user_id, name, npk, root_dose_ml_per_l, foliar_dose_ml_per_l, interval_days_active_season, interval_days_dormant_season)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		user.ID, name, npk, root, foliar, active, dormant).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			writeDetail(w, http.StatusConflict, "Удобрение с таким названием уже есть")
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	f, err := s.ownedFertilizer(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, fertSchema(f))
}

func (s *Server) getFertilizer(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	f, err := s.ownedFertilizer(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Удобрение не найдено")
		return
	}
	writeJSON(w, http.StatusOK, fertSchema(f))
}

func (s *Server) updateFertilizer(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedFertilizer(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Удобрение не найдено")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	set := map[string]any{}
	if b.has("name") {
		set["name"] = strOr(b, "name", "")
	}
	if b.has("npk") {
		set["npk"] = strOr(b, "npk", "")
	}
	if b.has("root_dose_ml_per_l") {
		set["root_dose_ml_per_l"] = bFloat(b, "root_dose_ml_per_l")
	}
	if b.has("foliar_dose_ml_per_l") {
		set["foliar_dose_ml_per_l"] = bFloat(b, "foliar_dose_ml_per_l")
	}
	if b.has("interval_days_active_season") {
		set["interval_days_active_season"] = intOr(b, "interval_days_active_season", 0)
	}
	if b.has("interval_days_dormant_season") {
		set["interval_days_dormant_season"] = bInt(b, "interval_days_dormant_season")
	}
	if err := applySet(r.Context(), s.Pool, "fertilizer_types", id, set); err != nil {
		if isUniqueViolation(err) {
			writeDetail(w, http.StatusConflict, "Удобрение с таким названием уже есть")
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	f, err := s.ownedFertilizer(r.Context(), id, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fertSchema(f))
}

func (s *Server) deleteFertilizer(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedFertilizer(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Удобрение не найдено")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM fertilizer_types WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func bInt(b body, key string) *int {
	v, _ := b.int(key)
	return v
}
