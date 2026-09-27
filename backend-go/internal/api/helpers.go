package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/render"
)

type detail map[string]any

// writeJSON — записи JSON-ответа.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if v == nil {
		_, _ = w.Write([]byte("null"))
		return
	}
	_ = enc.Encode(v)
}

// writeDetail — ответ как FastAPI: {"detail": "..."}.
func writeDetail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, detailStream(msg))
}

func detailStream(msg string) map[string]any {
	return map[string]any{"detail": msg}
}

// writeValidation — Pydantic 422 со списком ошибок.
func writeValidation(w http.ResponseWriter, errs []any) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"detail": errs})
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	return json.NewDecoder(r.Body).Decode(dst)
}

// applySet — PATCH-обновление записи: меняем только переданные колонки (явный null тоже).
func applySet(ctx context.Context, pool *pgxpool.Pool, table string, id int, set map[string]any) error {
	if len(set) == 0 {
		return nil
	}
	var cols []string
	for c := range set {
		cols = append(cols, c)
	}
	var sb strings.Builder
	sb.WriteString("UPDATE " + table + " SET ")
	args := make([]any, 0, len(set)+1)
	for i, c := range cols {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(c + " = $" + strconv.Itoa(i+1))
		args = append(args, set[c])
	}
	sb.WriteString(" WHERE id = $" + strconv.Itoa(len(cols)+1))
	args = append(args, id)
	_, err := pool.Exec(ctx, sb.String(), args...)
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func optTime(t *time.Time, zone *time.Location) *string {
	if t == nil {
		return nil
	}
	s := render.Time(*t, zone)
	return &s
}

func isIntegrity(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" || pgErr.Code == "23514"
	}
	return false
}
