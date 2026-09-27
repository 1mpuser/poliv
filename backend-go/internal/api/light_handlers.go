package api

import (
	"net/http"

	"poliv/internal/light"
	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
)

func (s *Server) geocode(w http.ResponseWriter, r *http.Request, user *models.User) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 || len(q) > 100 {
		writeValidation(w, nil)
		return
	}
	places, err := light.Geocode(q)
	if err != nil {
		writeDetail(w, http.StatusBadGateway, "Сервис поиска городов недоступен, попробуйте позже")
		return
	}
	out := make([]schema.Place, 0, len(places))
	for _, p := range places {
		out = append(out, schema.Place{Name: p.Name, Region: p.Region, Country: p.Country,
			Latitude: render.F(p.Latitude), Longitude: render.F(p.Longitude)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) lightToday(w http.ResponseWriter, r *http.Request, user *models.User) {
	day, err := s.currentDaylight(r.Context(), user.ID)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if day == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, schema.DaylightOut{
		Day: render.Date(day.Day), Sunrise: optTime(day.Sunrise, s.Zone), Sunset: optTime(day.Sunset, s.Zone),
		DaylightHours: render.F(day.DaylightHours), SunshineHours: render.F(day.SunshineHours),
	})
}
