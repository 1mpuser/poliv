package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/summary"
)

// ---------- Полив ----------

func (s *Server) listWaterings(w http.ResponseWriter, r *http.Request, user *models.User) {
	q := `SELECT w.id, w.plant_id, w.watered_at, w.note FROM watering_logs w
		JOIN plants p ON p.id=w.plant_id WHERE p.user_id=$1`
	args := []any{user.ID}
	if pid := r.URL.Query().Get("plant_id"); pid != "" {
		q += " AND w.plant_id=$2"
		n, _ := parseInt(pid)
		args = append(args, n)
	}
	q += " ORDER BY w.watered_at DESC"
	rows, err := s.Pool.Query(r.Context(), q, args...)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []schema.WateringOut{}
	for rows.Next() {
		var log models.WateringLog
		if err := rows.Scan(&log.ID, &log.PlantID, &log.WateredAt, &log.Note); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, schema.WateringOut{ID: log.ID, PlantID: log.PlantID, WateredAt: render.Time(log.WateredAt, s.Zone), Note: log.Note})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createWatering(w http.ResponseWriter, r *http.Request, user *models.User) {
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
	wateredAt, err := b.timeField("watered_at")
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if wateredAt == nil {
		t := time.Now().UTC()
		wateredAt = &t
	}
	note := bStr(b, "note")
	var id int
	if err := s.Pool.QueryRow(r.Context(), `INSERT INTO watering_logs (plant_id, watered_at, note) VALUES ($1,$2,$3) RETURNING id`,
		plantID, *wateredAt, note).Scan(&id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, schema.WateringOut{ID: id, PlantID: plantID, WateredAt: render.Time(*wateredAt, s.Zone), Note: note})
}

func (s *Server) getWatering(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `SELECT w.id, w.plant_id, w.watered_at, w.note FROM watering_logs w
		JOIN plants p ON p.id=w.plant_id WHERE w.id=$1 AND p.user_id=$2`, id, user.ID)
	var log models.WateringLog
	if err := row.Scan(&log.ID, &log.PlantID, &log.WateredAt, &log.Note); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	writeJSON(w, http.StatusOK, schema.WateringOut{ID: log.ID, PlantID: log.PlantID, WateredAt: render.Time(log.WateredAt, s.Zone), Note: log.Note})
}

func (s *Server) updateWatering(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.rLog(r.Context(), "watering_logs", id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	set := map[string]any{}
	if b.has("watered_at") {
		t, err := b.timeField("watered_at")
		if err != nil {
			writeValidation(w, nil)
			return
		}
		set["watered_at"] = t
	}
	if b.has("note") {
		set["note"] = bStr(b, "note")
	}
	if err := applySet(r.Context(), s.Pool, "watering_logs", id, set); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	row := s.Pool.QueryRow(r.Context(), `SELECT w.id, w.plant_id, w.watered_at, w.note FROM watering_logs w
		JOIN plants p ON p.id=w.plant_id WHERE w.id=$1 AND p.user_id=$2`, id, user.ID)
	var log models.WateringLog
	if err := row.Scan(&log.ID, &log.PlantID, &log.WateredAt, &log.Note); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	writeJSON(w, http.StatusOK, schema.WateringOut{ID: log.ID, PlantID: log.PlantID, WateredAt: render.Time(log.WateredAt, s.Zone), Note: log.Note})
}

func (s *Server) deleteWatering(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.rLog(r.Context(), "watering_logs", id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM watering_logs WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---------- Подкормка ----------

func (s *Server) listFeedings(w http.ResponseWriter, r *http.Request, user *models.User) {
	q := `SELECT f.id, f.plant_id, f.fertilizer_type_id, f.method::text, f.fed_at, f.note FROM feeding_logs f
		JOIN plants p ON p.id=f.plant_id WHERE p.user_id=$1`
	args := []any{user.ID}
	if pid := r.URL.Query().Get("plant_id"); pid != "" {
		q += " AND f.plant_id=$2"
		n, _ := parseInt(pid)
		args = append(args, n)
	}
	q += " ORDER BY f.fed_at DESC"
	rows, err := s.Pool.Query(r.Context(), q, args...)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []schema.FeedingOut{}
	for rows.Next() {
		var log models.FeedingLog
		if err := rows.Scan(&log.ID, &log.PlantID, &log.FertilizerTypeID, &log.Method, &log.FedAt, &log.Note); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, schema.FeedingOut{ID: log.ID, PlantID: log.PlantID, FertilizerTypeID: log.FertilizerTypeID,
			Method: log.Method, FedAt: render.Time(log.FedAt, s.Zone), Note: log.Note})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createFeeding(w http.ResponseWriter, r *http.Request, user *models.User) {
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
	fertID := intOr(b, "fertilizer_type_id", 0)
	if _, err := s.ownedFertilizer(r.Context(), fertID, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Удобрение не найдено")
		return
	}
	method := strOr(b, "method", "root")
	fedAt, err := b.timeField("fed_at")
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if fedAt == nil {
		t := time.Now().UTC()
		fedAt = &t
	}
	note := bStr(b, "note")

	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	// Корневая подкормка — всегда полив раствором. Полив создаём в той же транзакции,
	// но не плодим дубль: если в этот календарный день полив уже был, автополив не нужен.
	var wateringID *int
	if method == "root" {
		exists, err := s.wateringExistsInDay(ctx, tx, plantID, *fedAt)
		if err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !exists {
			wid, err := s.createAutoWatering(ctx, tx, plantID, *fedAt)
			if err != nil {
				writeDetail(w, http.StatusInternalServerError, err.Error())
				return
			}
			wateringID = &wid
		}
	}

	var id int
	if err := tx.QueryRow(ctx, `INSERT INTO feeding_logs (plant_id, fertilizer_type_id, method, fed_at, note, watering_id)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, plantID, fertID, method, *fedAt, note, wateringID).Scan(&id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, schema.FeedingOut{ID: id, PlantID: plantID, FertilizerTypeID: &fertID,
		Method: method, FedAt: render.Time(*fedAt, s.Zone), Note: note})
}

func (s *Server) getFeeding(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `SELECT f.id, f.plant_id, f.fertilizer_type_id, f.method::text, f.fed_at, f.note
		FROM feeding_logs f JOIN plants p ON p.id=f.plant_id WHERE f.id=$1 AND p.user_id=$2`, id, user.ID)
	var log models.FeedingLog
	if err := row.Scan(&log.ID, &log.PlantID, &log.FertilizerTypeID, &log.Method, &log.FedAt, &log.Note); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	writeJSON(w, http.StatusOK, schema.FeedingOut{ID: log.ID, PlantID: log.PlantID, FertilizerTypeID: log.FertilizerTypeID,
		Method: log.Method, FedAt: render.Time(log.FedAt, s.Zone), Note: log.Note})
}

func (s *Server) updateFeeding(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	ctx := r.Context()
	row := s.Pool.QueryRow(ctx, `SELECT f.id, f.plant_id, f.fertilizer_type_id, f.method::text, f.fed_at, f.note, f.watering_id
		FROM feeding_logs f JOIN plants p ON p.id=f.plant_id WHERE f.id=$1 AND p.user_id=$2`, id, user.ID)
	var log models.FeedingLog
	if err := row.Scan(&log.ID, &log.PlantID, &log.FertilizerTypeID, &log.Method, &log.FedAt, &log.Note, &log.WateringID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	set := map[string]any{}
	if b.has("fertilizer_type_id") {
		fid := intOr(b, "fertilizer_type_id", 0)
		if _, err := s.ownedFertilizer(ctx, fid, user.ID); err != nil {
			writeDetail(w, http.StatusNotFound, "Удобрение не найдено")
			return
		}
		set["fertilizer_type_id"] = fid
	}
	newMethod := log.Method
	if b.has("method") {
		newMethod = strOr(b, "method", "root")
		set["method"] = newMethod
	}
	newFedAt := log.FedAt
	if b.has("fed_at") {
		t, err := b.timeField("fed_at")
		if err != nil {
			writeValidation(w, nil)
			return
		}
		if t != nil {
			newFedAt = *t
		}
		set["fed_at"] = t
	}
	if b.has("note") {
		set["note"] = bStr(b, "note")
	}
	if err := applySet(ctx, s.Pool, "feeding_logs", id, set); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if newMethod != log.Method || !newFedAt.Equal(log.FedAt) {
		if err := s.reconcileFeedingWatering(ctx, log.PlantID, id, log.WateringID, newMethod, newFedAt); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	row = s.Pool.QueryRow(ctx, `SELECT f.id, f.plant_id, f.fertilizer_type_id, f.method::text, f.fed_at, f.note, f.watering_id
		FROM feeding_logs f JOIN plants p ON p.id=f.plant_id WHERE f.id=$1 AND p.user_id=$2`, id, user.ID)
	var upd models.FeedingLog
	if err := row.Scan(&upd.ID, &upd.PlantID, &upd.FertilizerTypeID, &upd.Method, &upd.FedAt, &upd.Note, &upd.WateringID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	writeJSON(w, http.StatusOK, schema.FeedingOut{ID: upd.ID, PlantID: upd.PlantID, FertilizerTypeID: upd.FertilizerTypeID,
		Method: upd.Method, FedAt: render.Time(upd.FedAt, s.Zone), Note: upd.Note})
}

func (s *Server) deleteFeeding(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	ctx := r.Context()
	var wateringID *int
	if err := s.Pool.QueryRow(ctx, `SELECT f.watering_id FROM feeding_logs f
		JOIN plants p ON p.id=f.plant_id WHERE f.id=$1 AND p.user_id=$2`, id, user.ID).Scan(&wateringID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM feeding_logs WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if wateringID != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM watering_logs WHERE id=$1`, *wateringID); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// querier — общий интерфейс для *pgxpool.Pool и pgx.Tx, чтобы проверки работали и в транзакции.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const autoWaterNote = "с подкормкой"

// wateringExistsInDay — есть ли у растения полив в тот же календарный день (в зоне учётки), что at.
// excludeID позволяет не считать сам автополив при проверке нового дня.
func (s *Server) wateringExistsInDay(ctx context.Context, q querier, plantID int, at time.Time) (bool, error) {
	return s.wateringExistsInDayExclude(ctx, q, plantID, at, nil)
}

func (s *Server) wateringExistsInDayExclude(ctx context.Context, q querier, plantID int, at time.Time, excludeID *int) (bool, error) {
	day := summary.LocalDate(at, s.Zone)
	start := summary.LocalMidnight(day, s.Zone)
	end := summary.LocalMidnight(summary.DateAddDays(day, 1), s.Zone)
	sql := `SELECT EXISTS(SELECT 1 FROM watering_logs WHERE plant_id=$1 AND watered_at >= $2 AND watered_at < $3`
	args := []any{plantID, start, end}
	if excludeID != nil {
		sql += ` AND id <> $4`
		args = append(args, *excludeID)
	}
	sql += `)`
	var exists bool
	if err := q.QueryRow(ctx, sql, args...).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// createAutoWatering — запись полива с заметкой о подкормке, создаётся в транзакции.
func (s *Server) createAutoWatering(ctx context.Context, tx pgx.Tx, plantID int, at time.Time) (int, error) {
	var id int
	err := tx.QueryRow(ctx, `INSERT INTO watering_logs (plant_id, watered_at, note) VALUES ($1,$2,$3) RETURNING id`,
		plantID, at, autoWaterNote).Scan(&id)
	return id, err
}

// reconcileFeedingWatering — после изменения подкормки согласует автополив.
// root→не-root удаляет автополив, не-root→root создаёт его, время двигается вместе с подкормкой.
// Вручную созданные поливы не затрагивает: ссылка watering_id есть только у автополива.
func (s *Server) reconcileFeedingWatering(ctx context.Context, plantID, feedingID int, oldWateringID *int, newMethod string, newFedAt time.Time) error {
	if newMethod != "root" {
		if oldWateringID == nil {
			return nil
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `UPDATE feeding_logs SET watering_id=NULL WHERE id=$1`, feedingID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM watering_logs WHERE id=$1`, *oldWateringID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if oldWateringID != nil {
		other, err := s.wateringExistsInDayExclude(ctx, tx, plantID, newFedAt, oldWateringID)
		if err != nil {
			return err
		}
		if other {
			if _, err := tx.Exec(ctx, `UPDATE feeding_logs SET watering_id=NULL WHERE id=$1`, feedingID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM watering_logs WHERE id=$1`, *oldWateringID); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		if _, err := tx.Exec(ctx, `UPDATE watering_logs SET watered_at=$2 WHERE id=$1`, *oldWateringID, newFedAt); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	exists, err := s.wateringExistsInDay(ctx, tx, plantID, newFedAt)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	wid, err := s.createAutoWatering(ctx, tx, plantID, newFedAt)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE feeding_logs SET watering_id=$2 WHERE id=$1`, feedingID, wid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------- Пересадка ----------

func (s *Server) listRepottings(w http.ResponseWriter, r *http.Request, user *models.User) {
	q := `SELECT r.id, r.plant_id, r.repotted_at, r.note, r.pot_size_before, r.pot_size_after FROM repotting_logs r
		JOIN plants p ON p.id=r.plant_id WHERE p.user_id=$1`
	args := []any{user.ID}
	if pid := r.URL.Query().Get("plant_id"); pid != "" {
		q += " AND r.plant_id=$2"
		n, _ := parseInt(pid)
		args = append(args, n)
	}
	q += " ORDER BY r.repotted_at DESC"
	rows, err := s.Pool.Query(r.Context(), q, args...)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []schema.RepottingOut{}
	for rows.Next() {
		var log models.RepottingLog
		if err := rows.Scan(&log.ID, &log.PlantID, &log.RepottedAt, &log.Note, &log.PotSizeBefore, &log.PotSizeAfter); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, schema.RepottingOut{ID: log.ID, PlantID: log.PlantID, RepottedAt: render.Time(log.RepottedAt, s.Zone),
			Note: log.Note, PotSizeBefore: render.MarshalF(log.PotSizeBefore), PotSizeAfter: render.MarshalF(log.PotSizeAfter)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createRepotting(w http.ResponseWriter, r *http.Request, user *models.User) {
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	plantID := intOr(b, "plant_id", 0)
	plant, err := s.ownedPlant(r.Context(), plantID, user.ID)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Растение не найдено")
		return
	}
	repottedAt, err := b.timeField("repotted_at")
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if repottedAt == nil {
		t := time.Now().UTC()
		repottedAt = &t
	}
	before := bFloat(b, "pot_size_before")
	if before == nil {
		before = plant.PotSizeL
	}
	after := bFloat(b, "pot_size_after")
	note := bStr(b, "note")
	var id int
	if err := s.Pool.QueryRow(r.Context(), `INSERT INTO repotting_logs (plant_id, repotted_at, pot_size_before, pot_size_after, note)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`, plantID, *repottedAt, before, after, note).Scan(&id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if after != nil {
		if _, err := s.Pool.Exec(r.Context(), `UPDATE plants SET pot_size_l=$1 WHERE id=$2`, *after, plantID); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, schema.RepottingOut{ID: id, PlantID: plantID, RepottedAt: render.Time(*repottedAt, s.Zone),
		Note: note, PotSizeBefore: render.MarshalF(before), PotSizeAfter: render.MarshalF(after)})
}

func (s *Server) getRepotting(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `SELECT r.id, r.plant_id, r.repotted_at, r.note, r.pot_size_before, r.pot_size_after
		FROM repotting_logs r JOIN plants p ON p.id=r.plant_id WHERE r.id=$1 AND p.user_id=$2`, id, user.ID)
	var log models.RepottingLog
	if err := row.Scan(&log.ID, &log.PlantID, &log.RepottedAt, &log.Note, &log.PotSizeBefore, &log.PotSizeAfter); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	writeJSON(w, http.StatusOK, schema.RepottingOut{ID: log.ID, PlantID: log.PlantID, RepottedAt: render.Time(log.RepottedAt, s.Zone),
		Note: log.Note, PotSizeBefore: render.MarshalF(log.PotSizeBefore), PotSizeAfter: render.MarshalF(log.PotSizeAfter)})
}

func (s *Server) updateRepotting(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.rLog(r.Context(), "repotting_logs", id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	set := map[string]any{}
	if b.has("repotted_at") {
		t, err := b.timeField("repotted_at")
		if err != nil {
			writeValidation(w, nil)
			return
		}
		set["repotted_at"] = t
	}
	if b.has("pot_size_before") {
		set["pot_size_before"] = bFloat(b, "pot_size_before")
	}
	if b.has("pot_size_after") {
		set["pot_size_after"] = bFloat(b, "pot_size_after")
	}
	if b.has("note") {
		set["note"] = bStr(b, "note")
	}
	if err := applySet(r.Context(), s.Pool, "repotting_logs", id, set); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schema.RepottingOut{ID: id})
}

func (s *Server) deleteRepotting(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if _, err := s.rLog(r.Context(), "repotting_logs", id, user.ID); err != nil {
		writeDetail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM repotting_logs WHERE id=$1`, id); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// rLog — проверка, что запись лога принадлежит учётке пользователя (через растение).
func (s *Server) rLog(ctx context.Context, table string, id, userID int) (bool, error) {
	col := "plant_id"
	row := s.Pool.QueryRow(ctx, `SELECT p.id FROM `+table+` t JOIN plants p ON p.id=t.`+col+` WHERE t.id=$1 AND p.user_id=$2`, id, userID)
	var pid int
	err := row.Scan(&pid)
	if err != nil {
		return false, err
	}
	return true, nil
}
