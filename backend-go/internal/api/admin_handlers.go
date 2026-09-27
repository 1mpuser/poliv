package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/users"
)

func adminUserOut(u *models.User, zone *time.Location) schema.AdminUser {
	out := schema.AdminUser{ID: u.ID, Email: u.Email, IsAdmin: u.IsAdmin, CreatedAt: render.Time(u.CreatedAt, zone)}
	if u.BlockedAt != nil {
		s := render.Time(*u.BlockedAt, zone)
		out.BlockedAt = &s
	}
	return out
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request, admin *models.User) {
	rows, err := s.Pool.Query(r.Context(), `SELECT id, email, password_hash, is_admin, blocked_at, token_version, created_at
		FROM users ORDER BY created_at, id`)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []schema.AdminUser{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.BlockedAt, &u.TokenVersion, &u.CreatedAt); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, adminUserOut(&u, s.Zone))
	}
	writeJSON(w, http.StatusOK, out)
}

type userCreate struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request, admin *models.User) {
	var b userCreate
	if err := decodeJSON(r, &b); err != nil {
		writeValidation(w, nil)
		return
	}
	if !strings.Contains(b.Email, "@") || len(strings.TrimSpace(b.Email)) > 254 {
		writeDetail(w, http.StatusBadRequest, "Укажите почту")
		return
	}
	u, err := users.CreateUser(r.Context(), s.Pool, b.Email, b.Password, false)
	if err != nil {
		if err == users.ErrEmailExists {
			writeDetail(w, http.StatusConflict, "Учётка с такой почтой уже есть")
			return
		}
		if strings.HasPrefix(err.Error(), "Пароль") {
			writeDetail(w, http.StatusBadRequest, err.Error())
			return
		}
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, adminUserOut(u, s.Zone))
}

type passwordSet struct {
	Password string `json:"password"`
}

func (s *Server) adminSetPassword(w http.ResponseWriter, r *http.Request, admin *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	u, err := s.adminUser(r.Context(), id)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Учётка не найдена")
		return
	}
	var b passwordSet
	if err := decodeJSON(r, &b); err != nil {
		writeValidation(w, nil)
		return
	}
	if err := users.SetPassword(r.Context(), s.Pool, u, b.Password); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) adminBlock(w http.ResponseWriter, r *http.Request, admin *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	u, err := s.adminUser(r.Context(), id)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Учётка не найдена")
		return
	}
	if u.ID == admin.ID {
		writeDetail(w, http.StatusBadRequest, "Нельзя заблокировать или удалить свою учётку")
		return
	}
	if err := users.SetBlocked(r.Context(), s.Pool, u, true); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) adminUnblock(w http.ResponseWriter, r *http.Request, admin *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	u, err := s.adminUser(r.Context(), id)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Учётка не найдена")
		return
	}
	if err := users.SetBlocked(r.Context(), s.Pool, u, false); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request, admin *models.User) {
	id, err := pathID(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	u, err := s.adminUser(r.Context(), id)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "Учётка не найдена")
		return
	}
	if u.ID == admin.ID {
		writeDetail(w, http.StatusBadRequest, "Нельзя заблокировать или удалить свою учётку")
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM users WHERE id=$1`, u.ID); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) adminUser(ctx context.Context, id int) (*models.User, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, email, password_hash, is_admin, blocked_at, token_version, created_at
		FROM users WHERE id=$1`, id)
	var u models.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.BlockedAt, &u.TokenVersion, &u.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound
		}
		return nil, err
	}
	return &u, nil
}
