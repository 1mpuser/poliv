package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/auth"
	"poliv/internal/db"
	"poliv/internal/lamps"
	"poliv/internal/models"
	"poliv/internal/plants"
	"poliv/internal/users"
)

type Server struct {
	Pool       *pgxpool.Pool
	Zone       *time.Location
	Secret     string
	ExpireDays int
	DB         *db.DB

	Lamps  *lamps.Service
	Plants *plants.Service
}

func New(pool *pgxpool.Pool, zone *time.Location, secret string, expireDays int) *Server {
	t := db.DB{Pool: pool, Zone: zone}
	lampSvc := lamps.New(pool, zone, secret)
	plantSvc := plants.New(pool, zone, lampSvc)
	return &Server{
		Pool: pool, Zone: zone, Secret: secret, ExpireDays: expireDays,
		DB: &t, Lamps: lampSvc, Plants: plantSvc,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/auth/token", s.login)
	mux.HandleFunc("GET /api/auth/me", s.wrap(s.me))
	mux.HandleFunc("POST /api/auth/password", s.wrap(s.changePassword))

	// админка
	mux.HandleFunc("GET /api/admin/users", s.wrapAdmin(s.adminListUsers))
	mux.HandleFunc("POST /api/admin/users", s.wrapAdmin(s.adminCreateUser))
	mux.HandleFunc("POST /api/admin/users/{id}/password", s.wrapAdmin(s.adminSetPassword))
	mux.HandleFunc("POST /api/admin/users/{id}/block", s.wrapAdmin(s.adminBlock))
	mux.HandleFunc("POST /api/admin/users/{id}/unblock", s.wrapAdmin(s.adminUnblock))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.wrapAdmin(s.adminDeleteUser))

	// растения
	mux.HandleFunc("GET /api/plants", s.wrap(s.listPlants))
	mux.HandleFunc("POST /api/plants", s.wrap(s.createPlant))
	mux.HandleFunc("GET /api/plants/summary", s.wrap(s.allSummaries))
	mux.HandleFunc("GET /api/plants/{id}/summary", s.wrap(s.plantSummary))
	mux.HandleFunc("GET /api/plants/{id}/history", s.wrap(s.plantHistory))
	mux.HandleFunc("GET /api/plants/{id}/stats/weekly", s.wrap(s.plantWeekly))
	mux.HandleFunc("GET /api/plants/{id}", s.wrap(s.getPlant))
	mux.HandleFunc("PATCH /api/plants/{id}", s.wrap(s.updatePlant))
	mux.HandleFunc("DELETE /api/plants/{id}", s.wrap(s.deletePlant))
	mux.HandleFunc("PUT /api/plants/{id}/lamp", s.wrap(s.setPlantLamp))

	// журналы
	mux.HandleFunc("GET /api/waterings", s.wrap(s.listWaterings))
	mux.HandleFunc("POST /api/waterings", s.wrap(s.createWatering))
	mux.HandleFunc("GET /api/waterings/{id}", s.wrap(s.getWatering))
	mux.HandleFunc("PATCH /api/waterings/{id}", s.wrap(s.updateWatering))
	mux.HandleFunc("DELETE /api/waterings/{id}", s.wrap(s.deleteWatering))

	mux.HandleFunc("GET /api/feedings", s.wrap(s.listFeedings))
	mux.HandleFunc("POST /api/feedings", s.wrap(s.createFeeding))
	mux.HandleFunc("GET /api/feedings/{id}", s.wrap(s.getFeeding))
	mux.HandleFunc("PATCH /api/feedings/{id}", s.wrap(s.updateFeeding))
	mux.HandleFunc("DELETE /api/feedings/{id}", s.wrap(s.deleteFeeding))

	mux.HandleFunc("GET /api/repottings", s.wrap(s.listRepottings))
	mux.HandleFunc("POST /api/repottings", s.wrap(s.createRepotting))
	mux.HandleFunc("GET /api/repottings/{id}", s.wrap(s.getRepotting))
	mux.HandleFunc("PATCH /api/repottings/{id}", s.wrap(s.updateRepotting))
	mux.HandleFunc("DELETE /api/repottings/{id}", s.wrap(s.deleteRepotting))

	// проверки грунта
	mux.HandleFunc("POST /api/plants/{id}/checks", s.wrap(s.createCheck))
	mux.HandleFunc("DELETE /api/checks/{id}", s.wrap(s.deleteCheck))

	// удобрения
	mux.HandleFunc("GET /api/fertilizers", s.wrap(s.listFertilizers))
	mux.HandleFunc("POST /api/fertilizers", s.wrap(s.createFertilizer))
	mux.HandleFunc("GET /api/fertilizers/{id}", s.wrap(s.getFertilizer))
	mux.HandleFunc("PATCH /api/fertilizers/{id}", s.wrap(s.updateFertilizer))
	mux.HandleFunc("DELETE /api/fertilizers/{id}", s.wrap(s.deleteFertilizer))

	// свет
	mux.HandleFunc("GET /api/light/geocode", s.wrap(s.geocode))
	mux.HandleFunc("GET /api/light/today", s.wrap(s.lightToday))

	// лампы
	mux.HandleFunc("GET /api/lamps", s.wrap(s.listLamps))
	mux.HandleFunc("POST /api/lamps", s.wrap(s.createLamp))
	mux.HandleFunc("GET /api/lamps/{id}", s.wrap(s.getLamp))
	mux.HandleFunc("PATCH /api/lamps/{id}", s.wrap(s.updateLamp))
	mux.HandleFunc("DELETE /api/lamps/{id}", s.wrap(s.archiveLamp))
	mux.HandleFunc("PUT /api/lamps/{id}/schedule", s.wrap(s.setLampSchedule))
	mux.HandleFunc("POST /api/lamps/{id}/toggle", s.wrap(s.toggleLamp))
	mux.HandleFunc("POST /api/lamps/{id}/pause", s.wrap(s.pauseLamp))
	mux.HandleFunc("DELETE /api/lamps/{id}/pause", s.wrap(s.resumeLamp))

	// сессии лампы
	mux.HandleFunc("GET /api/lamp-sessions", s.wrap(s.listSessions))
	mux.HandleFunc("POST /api/lamp-sessions", s.wrap(s.createSession))
	mux.HandleFunc("POST /api/lamp-sessions/toggle", s.wrap(s.toggleSession))
	mux.HandleFunc("GET /api/lamp-sessions/{id}", s.wrap(s.getSession))
	mux.HandleFunc("PATCH /api/lamp-sessions/{id}", s.wrap(s.updateSession))
	mux.HandleFunc("DELETE /api/lamp-sessions/{id}", s.wrap(s.deleteSession))

	// яндекс
	mux.HandleFunc("GET /api/yandex/devices", s.wrap(s.yandexDevices))

	// настройки
	mux.HandleFunc("GET /api/settings", s.wrap(s.readSettings))
	mux.HandleFunc("PATCH /api/settings", s.wrap(s.updateSettings))
	mux.HandleFunc("PUT /api/settings/yandex-token", s.wrap(s.setYandexToken))

	return mux
}

// ---------- auth-мидлвара ----------

type ctxUser struct{}

func (s *Server) wrap(next func(http.ResponseWriter, *http.Request, *models.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			writeDetail(w, http.StatusUnauthorized, "Сессия истекла, войдите снова")
			return
		}
		next(w, r, user)
	}
}

func (s *Server) wrapAdmin(next func(http.ResponseWriter, *http.Request, *models.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			writeDetail(w, http.StatusUnauthorized, "Сессия истекла, войдите снова")
			return
		}
		if !user.IsAdmin {
			writeDetail(w, http.StatusNotFound, "Not Found")
			return
		}
		next(w, r, user)
	}
}

func (s *Server) currentUser(r *http.Request) (*models.User, error) {
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		return nil, errorsNew("no token")
	}
	token := strings.TrimPrefix(authz, "Bearer ")
	userID, ver, err := auth.ParseToken(s.Secret, token)
	if err != nil {
		return nil, err
	}
	user, err := s.loadUser(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.BlockedAt != nil || ver != user.TokenVersion {
		return nil, errorsNew("bad user")
	}
	return user, nil
}

func (s *Server) loadUser(ctx context.Context, id int) (*models.User, error) {
	return users.GetByID(ctx, s.Pool, id)
}

func errorsNew(msg string) error { return &strErr{msg} }

type strErr struct{ msg string }

func (e *strErr) Error() string { return e.msg }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func pathID(r *http.Request) (int, error) {
	return strconv.Atoi(r.PathValue("id"))
}
