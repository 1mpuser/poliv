package api

import (
	"context"
	"errors"
	"net/http"

	"poliv/internal/light"
	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/secretbox"
	"poliv/internal/users"
	"poliv/internal/yandex"
)

func settingsOut(s *models.UserSettings) schema.SettingsOut {
	return schema.SettingsOut{
		CurrentSeason: s.CurrentSeason, NotifyDaysAhead: s.NotifyDaysAhead,
		LocationName: s.LocationName, Latitude: render.MarshalF(s.Latitude), Longitude: render.MarshalF(s.Longitude),
		YandexStatus: s.YandexStatus(),
	}
}

func (s *Server) readSettings(w http.ResponseWriter, r *http.Request, user *models.User) {
	row, err := users.GetUserSettings(r.Context(), s.Pool, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settingsOut(row))
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request, user *models.User) {
	row, err := users.GetUserSettings(r.Context(), s.Pool, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	before := place(row)
	b, err := readBody(r)
	if err != nil {
		writeValidation(w, nil)
		return
	}
	if b.has("current_season") {
		row.CurrentSeason = strOr(b, "current_season", "active")
	}
	if b.has("notify_days_ahead") {
		row.NotifyDaysAhead = intOr(b, "notify_days_ahead", 0)
	}
	if b.has("location_name") {
		row.LocationName = bStr(b, "location_name")
	}
	if b.has("latitude") {
		row.Latitude = bFloat(b, "latitude")
	}
	if b.has("longitude") {
		row.Longitude = bFloat(b, "longitude")
	}
	if err := saveSettings(r.Context(), s, row); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if changed := !samePlace(before, place(row)); changed {
		// новый город — свет по нему сразу; сеть подвела — догонит фоновая синхронизация
		_, _ = light.SyncDaylight(r.Context(), s.Pool, row, s.Zone, true)
	}
	out, err := users.GetUserSettings(r.Context(), s.Pool, user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settingsOut(out))
}

type yandexTokenSet struct {
	Token *string `json:"token"`
}

func (s *Server) setYandexToken(w http.ResponseWriter, r *http.Request, user *models.User) {
	var b yandexTokenSet
	if err := decodeJSON(r, &b); err != nil {
		writeValidation(w, nil)
		return
	}
	if b.Token == nil {
		if _, err := s.Pool.Exec(r.Context(), `UPDATE user_settings SET yandex_token=NULL, yandex_token_invalid=false WHERE user_id=$1`, user.ID); err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
		out, _ := users.GetUserSettings(r.Context(), s.Pool, user.ID)
		writeJSON(w, http.StatusOK, settingsOut(out))
		return
	}
	token := trimToken(*b.Token)
	if _, err := s.Lamps.ListDevices(token); err != nil {
		var authErr *yandex.YandexAuthError
		if ok := isAuthErr(err, &authErr); ok {
			writeDetail(w, http.StatusBadRequest, "Яндекс не принял токен — проверьте права «Умный дом» (iot:view, iot:control)")
			return
		}
		writeDetail(w, http.StatusBadGateway, err.Error())
		return
	}
	enc, err := secretbox.Encrypt(token, s.Secret)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `UPDATE user_settings SET yandex_token=$1, yandex_token_invalid=false WHERE user_id=$2`,
		enc, user.ID); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := users.GetUserSettings(r.Context(), s.Pool, user.ID)
	writeJSON(w, http.StatusOK, settingsOut(out))
}

func saveSettings(ctx context.Context, s *Server, row *models.UserSettings) error {
	_, err := s.Pool.Exec(ctx, `UPDATE user_settings SET current_season=$1::season, notify_days_ahead=$2,
		location_name=$3, latitude=$4, longitude=$5 WHERE user_id=$6`,
		row.CurrentSeason, row.NotifyDaysAhead, row.LocationName, row.Latitude, row.Longitude, row.UserID)
	return err
}

func place(row *models.UserSettings) [2]float64 {
	var lat, lon float64
	if row.Latitude != nil {
		lat = *row.Latitude
	}
	if row.Longitude != nil {
		lon = *row.Longitude
	}
	return [2]float64{lat, lon}
}

func samePlace(a, b [2]float64) bool {
	return a[0] == b[0] && a[1] == b[1]
}

func isAuthErr(err error, out **yandex.YandexAuthError) bool {
	return errors.As(err, out)
}

func trimToken(t string) string {
	start := 0
	end := len(t)
	for start < end && t[start] == ' ' {
		start++
	}
	for end > start && t[end-1] == ' ' {
		end--
	}
	return t[start:end]
}
