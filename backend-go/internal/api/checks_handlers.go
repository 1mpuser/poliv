package api

import (
	"net/http"
	"time"

	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
)

func (s *Server) createCheck(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.ownedPlant(r.Context(), id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	now := time.Now().UTC()
	var checkID int
	if err := s.Pool.QueryRow(r.Context(), `INSERT INTO soil_checks (plant_id, checked_at) VALUES ($1,$2) RETURNING id`,
		id, now).Scan(&checkID); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, schema.SoilCheckOut{ID: checkID, PlantID: id, CheckedAt: render.Time(now, s.Zone)})
}

func (s *Server) deleteCheck(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	var pid int
	err = s.Pool.QueryRow(r.Context(), `SELECT c.plant_id FROM soil_checks c JOIN plants p ON p.id=c.plant_id
		WHERE c.id=$1 AND p.user_id=$2`, id, user.ID).Scan(&pid)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM soil_checks WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
