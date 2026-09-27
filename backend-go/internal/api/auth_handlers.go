package api

import (
	"net/http"
	"time"

	"poliv/internal/auth"
	"poliv/internal/models"
	"poliv/internal/passwords"
	"poliv/internal/schema"
	"poliv/internal/users"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	user, err := users.FindByEmail(r.Context(), s.Pool, username)
	ok := false
	if err == nil && user != nil {
		ok = passwords.VerifyPassword(password, user.PasswordHash) && user.BlockedAt == nil
	}
	if !ok {
		time.Sleep(time.Second)
		writeDetail(w, http.StatusUnauthorized, "Неверная почта или пароль")
		return
	}
	token, err := auth.IssueToken(s.Secret, s.ExpireDays, user.ID, user.TokenVersion)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "Ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, schema.Token{AccessToken: token, TokenType: "bearer"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, user *models.User) {
	writeJSON(w, http.StatusOK, schema.Me{ID: user.ID, Email: user.Email, IsAdmin: user.IsAdmin})
}

type passwordChange struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, user *models.User) {
	var b passwordChange
	if err := decodeJSON(r, &b); err != nil {
		writeValidation(w, nil)
		return
	}
	if !passwords.VerifyPassword(b.CurrentPassword, user.PasswordHash) {
		writeDetail(w, http.StatusBadRequest, "Текущий пароль неверен")
		return
	}
	if err := users.SetPassword(r.Context(), s.Pool, user, b.NewPassword); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	user.TokenVersion++
	token, err := auth.IssueToken(s.Secret, s.ExpireDays, user.ID, user.TokenVersion)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "Ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, schema.Token{AccessToken: token, TokenType: "bearer"})
}
