package api

import (
	"errors"
	"net/http"

	"poliv/internal/models"
	"poliv/internal/schema"
	"poliv/internal/yandex"
)

func (s *Server) yandexDevices(w http.ResponseWriter, r *http.Request, user *models.User) {
	token, err := s.Lamps.YandexToken(r.Context(), user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if token == nil {
		writeDetail(w, http.StatusBadRequest, "Сначала вставьте токен Яндекса в разделе «Свет»")
		return
	}
	devices, err := s.Lamps.ListDevices(*token)
	if err != nil {
		var authErr *yandex.YandexAuthError
		if errors.As(err, &authErr) {
			if _, err := s.Pool.Exec(r.Context(), `UPDATE user_settings SET yandex_token_invalid=true WHERE user_id=$1`, user.ID); err != nil {
				writeDetail(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeDetail(w, http.StatusBadRequest, "Токен Яндекса недействителен — вставьте новый в разделе «Свет»")
			return
		}
		writeDetail(w, http.StatusBadGateway, err.Error())
		return
	}
	out := make([]schema.YandexDevice, 0, len(devices))
	for _, d := range devices {
		out = append(out, schema.YandexDevice{ID: d.ID, Name: d.Name, Room: d.Room, Type: d.Type})
	}
	writeJSON(w, http.StatusOK, out)
}
