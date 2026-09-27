package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/light"
	"poliv/internal/migrate"
	"poliv/internal/passwords"
	"poliv/internal/yandex"
)

var testBase string
var testPool *pgxpool.Pool
var testSrv *Server

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		os.Exit(m.Run())
	}
	if !strings.Contains(path.Base(url), "test") {
		fmt.Fprintln(os.Stderr, "TEST_DATABASE_URL должен указывать на базу с «test» в имени")
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		fmt.Fprintln(os.Stderr, "drop:", err)
		os.Exit(1)
	}
	if err := migrate.New(pool).Up(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	zone, _ := time.LoadLocation("Europe/Moscow")
	testPool = pool
	testSrv = New(pool, zone, "test-secret", 30)

	// сеть (Яндекс, Open-Meteo) в тестах не трогаем — по умолчанию заглушки
	testSrv.Lamps.YandexListDevices = func(token string) ([]yandex.Device, error) { return nil, nil }
	testSrv.Lamps.YandexSetOn = func(token, dev string, on bool) error { return nil }

	hash, _ := passwords.HashPassword("owner-pass-1")
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET email='owner@example.com', password_hash=$1, is_admin=true, blocked_at=NULL, token_version=1 WHERE id=1`, hash); err != nil {
		fmt.Fprintln(os.Stderr, "set owner:", err)
		os.Exit(1)
	}

	ts := httptest.NewServer(testSrv.Handler())
	testBase = ts.URL
	code := m.Run()
	ts.Close()
	os.Exit(code)
}

// ---------- helpers ----------

func req(t *testing.T, method, path string, body any, token string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, testBase+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		_ = json.Unmarshal(b, out)
	}
}

func emailToken(t *testing.T, email, password string) string {
	t.Helper()
	form := "username=" + email + "&password=" + password
	resp, err := http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded", strings.NewReader(form))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login %s -> %d", email, resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tok)
	return tok.AccessToken
}

func adminToken(t *testing.T) string {
	return emailToken(t, "owner@example.com", "owner-pass-1")
}

func makeUser(t *testing.T, admin, email, password string) (int, string) {
	t.Helper()
	if password == "" {
		password = "user-pass-1"
	}
	resp := req(t, "POST", "/api/admin/users", map[string]string{"email": email, "password": password}, admin)
	if resp.StatusCode != 201 {
		t.Fatalf("makeUser %s -> %d", email, resp.StatusCode)
	}
	var u struct {
		ID int `json:"id"`
	}
	decode(t, resp, &u)
	return u.ID, emailToken(t, email, password)
}

func getJSON(t *testing.T, path, token string, out any) int {
	t.Helper()
	resp := req(t, "GET", path, nil, token)
	if resp.StatusCode == 204 {
		resp.Body.Close()
		return 204
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		_ = json.Unmarshal(b, out)
	}
	return resp.StatusCode
}

// ---------- тесты ----------

func TestOwnerKeepsSeedData(t *testing.T) {
	admin := adminToken(t)
	var plants []map[string]any
	if status := getJSON(t, "/api/plants", admin, &plants); status != 200 {
		t.Fatalf("status %d", status)
	}
	if len(plants) != 2 || plants[0]["name"] != "Лимон" || plants[1]["name"] != "Лайм" {
		t.Fatalf("plants %v", plants)
	}
	var me map[string]any
	getJSON(t, "/api/auth/me", admin, &me)
	if me["email"] != "owner@example.com" || me["is_admin"] != true {
		t.Fatalf("me %v", me)
	}
}

func TestPlaceholderOwnerCannotLogin(t *testing.T) {
	resp, err := http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded",
		strings.NewReader("username=owner@localhost.invalid&password=!"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestNewUserStartsEmptyWithDefaults(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "fresh@example.com", "user-pass-1")
	var plants []map[string]any
	getJSON(t, "/api/plants", tok, &plants)
	if len(plants) != 0 {
		t.Fatalf("plants %v", plants)
	}
	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	if len(ferts) != 2 || ferts[0]["name"] != "Lomonosoff" || ferts[1]["name"] != "Bona Forte" {
		t.Fatalf("ferts %v", ferts)
	}
	var sett map[string]any
	getJSON(t, "/api/settings", tok, &sett)
	if sett["current_season"] != "active" {
		t.Fatalf("settings %v", sett)
	}
}

func TestDataIsolation(t *testing.T) {
	admin := adminToken(t)
	_, a := makeUser(t, admin, "a@example.com", "user-pass-1")
	_, b := makeUser(t, admin, "b@example.com", "user-pass-1")

	var plant map[string]any
	req(t, "POST", "/api/plants", map[string]any{"name": "Фикус A"}, a)
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Фикус A"}, a)
	decode(t, resp, &plant)
	var water map[string]any
	resp = req(t, "POST", "/api/waterings", map[string]any{"plant_id": plant["id"]}, a)
	decode(t, resp, &water)
	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", a, &ferts)
	fertA := ferts[0]

	var plantsB []map[string]any
	getJSON(t, "/api/plants", b, &plantsB)
	if len(plantsB) != 0 {
		t.Fatalf("b plants %v", plantsB)
	}

	pid := int(plant["id"].(float64))
	wid := int(water["id"].(float64))
	fid := int(fertA["id"].(float64))
	for _, tc := range []struct{ method, path string; body any }{
		{"GET", fmt.Sprintf("/api/plants/%d", pid), nil},
		{"GET", fmt.Sprintf("/api/plants/%d/summary", pid), nil},
		{"GET", fmt.Sprintf("/api/plants/%d/history", pid), nil},
		{"PATCH", fmt.Sprintf("/api/plants/%d", pid), map[string]any{"name": "взлом"}},
		{"DELETE", fmt.Sprintf("/api/plants/%d", pid), nil},
		{"POST", "/api/waterings", map[string]any{"plant_id": pid}},
		{"DELETE", fmt.Sprintf("/api/waterings/%d", wid), nil},
		{"POST", "/api/lamp-sessions/toggle", map[string]any{"plant_id": pid}},
		{"PATCH", fmt.Sprintf("/api/fertilizers/%d", fid), map[string]any{"name": "взлом"}},
	} {
		resp := req(t, tc.method, tc.path, tc.body, b)
		if resp.StatusCode != 404 {
			t.Fatalf("%s %s -> %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	var watersB []map[string]any
	getJSON(t, "/api/waterings", b, &watersB)
	if len(watersB) != 0 {
		t.Fatalf("b waters %v", watersB)
	}

	// подкормка своего растения чужим удобрением
	var plantB map[string]any
	resp = req(t, "POST", "/api/plants", map[string]any{"name": "Фикус B"}, b)
	decode(t, resp, &plantB)
	resp = req(t, "POST", "/api/feedings", map[string]any{"plant_id": plantB["id"], "fertilizer_type_id": fid, "method": "root"}, b)
	if resp.StatusCode != 404 {
		t.Fatalf("feed foreign fert -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	// лампы
	var lampA map[string]any
	resp = req(t, "POST", "/api/lamps", map[string]any{"name": "Лампа A", "plant_ids": []int{pid}}, a)
	decode(t, resp, &lampA)
	lampID := int(lampA["id"].(float64))
	for _, tc := range []struct{ method, path string; body any }{
		{"GET", fmt.Sprintf("/api/lamps/%d", lampID), nil},
		{"PATCH", fmt.Sprintf("/api/lamps/%d", lampID), map[string]any{"name": "взлом"}},
		{"DELETE", fmt.Sprintf("/api/lamps/%d", lampID), nil},
		{"POST", fmt.Sprintf("/api/lamps/%d/toggle", lampID), nil},
		{"PUT", fmt.Sprintf("/api/lamps/%d/schedule", lampID), map[string]any{"intervals": []any{}}},
		{"PUT", fmt.Sprintf("/api/plants/%d/lamp", int(plantB["id"].(float64))), map[string]any{"lamp_id": lampID}},
		{"POST", "/api/lamps", map[string]any{"name": "чужое растение", "plant_ids": []int{pid}}},
		{"POST", "/api/lamp-sessions", map[string]any{"lamp_id": lampID}},
	} {
		resp := req(t, tc.method, tc.path, tc.body, b)
		if resp.StatusCode != 404 {
			t.Fatalf("%s %s -> %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	var lampsB []map[string]any
	getJSON(t, "/api/lamps", b, &lampsB)
	if len(lampsB) != 0 {
		t.Fatalf("b lamps %v", lampsB)
	}
	resp = req(t, "POST", fmt.Sprintf("/api/lamps/%d/toggle", lampID), nil, a)
	var toggleA map[string]any
	decode(t, resp, &toggleA)
	if toggleA["is_on"] != true {
		t.Fatalf("toggle %v", toggleA)
	}
	var sumA map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), a, &sumA)
	lampS, _ := sumA["lamp"].(map[string]any)
	if lampS["is_on"] != true {
		t.Fatalf("summary lamp %v", lampS)
	}
}

func TestAdminEndpointsHiddenFromUsers(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "plain@example.com", "user-pass-1")
	resp := req(t, "GET", "/api/admin/users", nil, tok)
	if resp.StatusCode != 404 {
		t.Fatalf("admin list -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "POST", "/api/admin/users", map[string]any{"email": "x@y.z", "password": "12345678"}, tok)
	if resp.StatusCode != 404 {
		t.Fatalf("admin create -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCreateValidation(t *testing.T) {
	admin := adminToken(t)
	resp := req(t, "POST", "/api/admin/users", map[string]any{"email": "A@EXAMPLE.com", "password": "12345678"}, admin)
	if resp.StatusCode != 409 {
		t.Fatalf("dup -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "POST", "/api/admin/users", map[string]any{"email": "short2@example.com", "password": "123"}, admin)
	if resp.StatusCode != 400 {
		t.Fatalf("short -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "POST", "/api/admin/users", map[string]any{"email": "not-an-email", "password": "12345678"}, admin)
	if resp.StatusCode != 400 {
		t.Fatalf("no email -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBlockUnblockRevokesTokens(t *testing.T) {
	admin := adminToken(t)
	uid, tok := makeUser(t, admin, "blocked@example.com", "user-pass-1")
	resp := req(t, "POST", fmt.Sprintf("/api/admin/users/%d/block", uid), nil, admin)
	if resp.StatusCode != 204 {
		t.Fatalf("block -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "GET", "/api/plants", nil, tok)
	if resp.StatusCode != 401 {
		t.Fatalf("after block -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded", strings.NewReader("username=blocked@example.com&password=user-pass-1"))
	if resp.StatusCode != 401 {
		t.Fatalf("login blocked -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	req(t, "POST", fmt.Sprintf("/api/admin/users/%d/unblock", uid), nil, admin)
	resp, _ = http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded", strings.NewReader("username=blocked@example.com&password=user-pass-1"))
	if resp.StatusCode != 200 {
		t.Fatalf("login after unblock -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminPasswordResetRevokesTokens(t *testing.T) {
	admin := adminToken(t)
	uid, tok := makeUser(t, admin, "reset@example.com", "user-pass-1")
	resp := req(t, "POST", fmt.Sprintf("/api/admin/users/%d/password", uid), map[string]any{"password": "new-pass-22"}, admin)
	if resp.StatusCode != 204 {
		t.Fatalf("reset -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "GET", "/api/plants", nil, tok)
	if resp.StatusCode != 401 {
		t.Fatalf("after reset -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded", strings.NewReader("username=reset@example.com&password=user-pass-1"))
	if resp.StatusCode != 401 {
		t.Fatalf("old pw -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded", strings.NewReader("username=reset@example.com&password=new-pass-22"))
	if resp.StatusCode != 200 {
		t.Fatalf("new pw -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminCannotBlockOrDeleteSelf(t *testing.T) {
	admin := adminToken(t)
	var me map[string]any
	getJSON(t, "/api/auth/me", admin, &me)
	uid := int(me["id"].(float64))
	resp := req(t, "POST", fmt.Sprintf("/api/admin/users/%d/block", uid), nil, admin)
	if resp.StatusCode != 400 {
		t.Fatalf("block self -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "DELETE", fmt.Sprintf("/api/admin/users/%d", uid), nil, admin)
	if resp.StatusCode != 400 {
		t.Fatalf("delete self -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDeleteUserRemovesData(t *testing.T) {
	admin := adminToken(t)
	uid, tok := makeUser(t, admin, "gone@example.com", "user-pass-1")
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Удалится"}, tok)
	decode(t, resp, &plant)
	pid := int(plant["id"].(float64))
	resp = req(t, "DELETE", fmt.Sprintf("/api/admin/users/%d", uid), nil, admin)
	if resp.StatusCode != 204 {
		t.Fatalf("delete -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	var cnt int
	_ = testPool.QueryRow(context.Background(), `SELECT count(*) FROM plants WHERE id=$1`, pid).Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("plant left %d", cnt)
	}
	resp, _ = http.Post(testBase+"/api/auth/token", "application/x-www-form-urlencoded", strings.NewReader("username=gone@example.com&password=user-pass-1"))
	if resp.StatusCode != 401 {
		t.Fatalf("login gone -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	var owners []map[string]any
	getJSON(t, "/api/plants", admin, &owners)
	if len(owners) != 2 {
		t.Fatalf("owners %d", len(owners))
	}
}

func TestChangeOwnPassword(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "self@example.com", "user-pass-1")
	resp := req(t, "POST", "/api/auth/password", map[string]any{"current_password": "wrong", "new_password": "brand-new-1"}, tok)
	if resp.StatusCode != 400 {
		t.Fatalf("bad current -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	ok := req(t, "POST", "/api/auth/password", map[string]any{"current_password": "user-pass-1", "new_password": "brand-new-1"}, tok)
	if ok.StatusCode != 200 {
		t.Fatalf("change -> %d", ok.StatusCode)
	}
	var tokOut struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, ok, &tokOut)
	resp = req(t, "GET", "/api/plants", nil, tok)
	if resp.StatusCode != 401 {
		t.Fatalf("old token -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "GET", "/api/plants", nil, tokOut.AccessToken)
	if resp.StatusCode != 200 {
		t.Fatalf("new token -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------- свет и лампы ----------

func TestSoilCheckCRUDAndStatusReset(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "check@example.com", "user-pass-1")
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Грунт"}, tok)
	decode(t, resp, &plant)
	pid := int(plant["id"].(float64))

	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	water, _ := sum["water"].(map[string]any)
	if water["status"] != "late" {
		t.Fatalf("water %v", water)
	}

	checkResp := req(t, "POST", fmt.Sprintf("/api/plants/%d/checks", pid), nil, tok)
	if checkResp.StatusCode != 201 {
		t.Fatalf("check -> %d", checkResp.StatusCode)
	}
	var check map[string]any
	decode(t, checkResp, &check)
	cid := int(check["id"].(float64))

	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	water, _ = sum["water"].(map[string]any)
	if water["status"] != "ok" || water["days_since"] != nil || water["days_since_check"] != float64(0) {
		t.Fatalf("water after check %v", water)
	}

	var hist []map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/history?types=check", pid), tok, &hist)
	if len(hist) != 1 || hist[0]["type"] != "check" {
		t.Fatalf("hist %v", hist)
	}

	resp = req(t, "DELETE", fmt.Sprintf("/api/checks/%d", cid), nil, tok)
	if resp.StatusCode != 204 {
		t.Fatalf("del check -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	water, _ = sum["water"].(map[string]any)
	if water["status"] != "late" {
		t.Fatalf("water after del %v", water)
	}
	hist = []map[string]any{}
	getJSON(t, fmt.Sprintf("/api/plants/%d/history?types=check", pid), tok, &hist)
	if len(hist) != 0 {
		t.Fatalf("hist after del %v", hist)
	}
}

func TestSoilCheckForeign404(t *testing.T) {
	admin := adminToken(t)
	_, a := makeUser(t, admin, "ca@example.com", "user-pass-1")
	_, b := makeUser(t, admin, "cb@example.com", "user-pass-1")
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "PA"}, a)
	decode(t, resp, &plant)
	pid := int(plant["id"].(float64))
	checkResp := req(t, "POST", fmt.Sprintf("/api/plants/%d/checks", pid), nil, a)
	var check map[string]any
	decode(t, checkResp, &check)
	cid := int(check["id"].(float64))

	resp = req(t, "POST", fmt.Sprintf("/api/plants/%d/checks", pid), nil, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign check -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "DELETE", fmt.Sprintf("/api/checks/%d", cid), nil, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign del -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "POST", fmt.Sprintf("/api/plants/%d/checks", pid), nil, a)
	if resp.StatusCode != 201 {
		t.Fatalf("own check -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func windowAroundNow(t *testing.T) (string, string) {
	zone, _ := time.LoadLocation("Europe/Moscow")
	now := time.Now().In(zone)
	start := now.Add(-time.Hour)
	if start.Hour() == 0 && start.Minute() == 0 && start.Second() == 0 {
		start = now.Add(-time.Hour)
	}
	end := now.Add(time.Hour)
	if end.Hour() == 23 {
		end = now
	}
	if start.Hour() == 0 {
		start = now
	}
	// не через полночь
	if end.Hour() < start.Hour() {
		end = start.Add(30 * time.Minute)
	}
	return start.Format("15:04"), end.Format("15:04")
}

func TestScheduleLampLightsItsPlantsAndToggleEndsIt(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "lamp@example.com", "user-pass-1")
	var lemon map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Лимон", "light_target_hours": 13}, tok)
	decode(t, resp, &lemon)
	var lime map[string]any
	resp = req(t, "POST", "/api/plants", map[string]any{"name": "Лайм"}, tok)
	decode(t, resp, &lime)
	var lamp map[string]any
	resp = req(t, "POST", "/api/lamps", map[string]any{"name": "Лампа цитрусы", "mode": "schedule", "plant_ids": []int{int(lemon["id"].(float64)), int(lime["id"].(float64))}}, tok)
	decode(t, resp, &lamp)
	lampID := int(lamp["id"].(float64))
	plants := lamp["plant_ids"].([]any)
	if len(plants) != 2 {
		t.Fatalf("plants %v", plants)
	}
	start, end := windowAroundNow(t)
	resp = req(t, "PUT", fmt.Sprintf("/api/lamps/%d/schedule", lampID), map[string]any{"intervals": []map[string]string{{"start_time": start, "end_time": end}}}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("schedule -> %d", resp.StatusCode)
	}
	decode(t, resp, nil)

	for _, p := range []map[string]any{lemon, lime} {
		var sum map[string]any
		getJSON(t, fmt.Sprintf("/api/plants/%d/summary", int(p["id"].(float64))), tok, &sum)
		lampS, _ := sum["lamp"].(map[string]any)
		lightS, _ := sum["light"].(map[string]any)
		if lampS["is_on"] != true {
			t.Fatalf("lamp not on %v", lampS)
		}
		if lightS["lamp_hours"].(float64) <= 0 {
			t.Fatalf("lamp_hours %v", lightS["lamp_hours"])
		}
		lampB, _ := lightS["lamp"].(map[string]any)
		if lampB["name"] != "Лампа цитрусы" {
			t.Fatalf("lamp brief %v", lampB)
		}
	}

	// кнопка у лимона гасит
	var off map[string]any
	resp = req(t, "POST", "/api/lamp-sessions/toggle", map[string]any{"plant_id": int(lemon["id"].(float64))}, tok)
	decode(t, resp, &off)
	if off["is_on"] != false || off["previous_ended_at"] == nil {
		t.Fatalf("off %v", off)
	}
	var limeSum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", int(lime["id"].(float64))), tok, &limeSum)
	limeLamp, _ := limeSum["lamp"].(map[string]any)
	if limeLamp["is_on"] != false {
		t.Fatalf("lime still on %v", limeLamp)
	}
	// отмена
	sess := off["session"].(map[string]any)
	prev := off["previous_ended_at"].(string)
	resp = req(t, "PATCH", fmt.Sprintf("/api/lamp-sessions/%d", int(sess["id"].(float64))), map[string]any{"ended_at": prev}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("restore -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", int(lime["id"].(float64))), tok, &limeSum)
	limeLamp, _ = limeSum["lamp"].(map[string]any)
	if limeLamp["is_on"] != true {
		t.Fatalf("lime after restore %v", limeLamp)
	}

	// повторная замена расписания без дублей, пустое — убирает сегодняшние
	resp = req(t, "PUT", fmt.Sprintf("/api/lamps/%d/schedule", lampID), map[string]any{"intervals": []map[string]string{{"start_time": start, "end_time": end}}}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("reschedule -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	var sessions []map[string]any
	getJSON(t, fmt.Sprintf("/api/lamp-sessions?lamp_id=%d", lampID), tok, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("sessions %v", sessions)
	}
	req(t, "PUT", fmt.Sprintf("/api/lamps/%d/schedule", lampID), map[string]any{"intervals": []any{}}, tok)
	sessions = []map[string]any{}
	getJSON(t, fmt.Sprintf("/api/lamp-sessions?lamp_id=%d", lampID), tok, &sessions)
	if len(sessions) != 0 {
		t.Fatalf("sessions after empty %v", sessions)
	}

	// расписание — только в режиме schedule
	req(t, "PATCH", fmt.Sprintf("/api/lamps/%d", lampID), map[string]any{"mode": "manual"}, tok)
	resp = req(t, "PUT", fmt.Sprintf("/api/lamps/%d/schedule", lampID), map[string]any{"intervals": []map[string]string{{"start_time": "18:00", "end_time": "22:00"}}}, tok)
	if resp.StatusCode != 400 {
		t.Fatalf("schedule in manual -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestLampValidation(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "sched@example.com", "user-pass-1")
	var lamp map[string]any
	resp := req(t, "POST", "/api/lamps", map[string]any{"name": "L", "mode": "schedule"}, tok)
	decode(t, resp, &lamp)
	lampID := int(lamp["id"].(float64))
	for _, ivs := range [][]map[string]string{
		{{"start_time": "10:00", "end_time": "09:00"}},
		{{"start_time": "07:00", "end_time": "10:00"}, {"start_time": "09:00", "end_time": "12:00"}},
	} {
		resp = req(t, "PUT", fmt.Sprintf("/api/lamps/%d/schedule", lampID), map[string]any{"intervals": ivs}, tok)
		if resp.StatusCode != 400 {
			t.Fatalf("bad schedule -> %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp = req(t, "PATCH", fmt.Sprintf("/api/lamps/%d", lampID), map[string]any{"morning_not_before": "23:00", "evening_not_after": "06:00"}, tok)
	if resp.StatusCode != 400 {
		t.Fatalf("bounds -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "POST", "/api/lamps", map[string]any{"name": "A", "mode": "auto"}, tok)
	if resp.StatusCode != 400 {
		t.Fatalf("auto without city -> %d", resp.StatusCode)
	}
	var detail map[string]any
	decode(t, resp, &detail)
	if !strings.Contains(detail["detail"].(string), "город") {
		t.Fatalf("detail %v", detail)
	}
}

func TestPlantWithoutLampCannotToggle(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "nolamp@example.com", "user-pass-1")
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "P"}, tok)
	decode(t, resp, &plant)
	pid := int(plant["id"].(float64))
	resp = req(t, "POST", "/api/lamp-sessions/toggle", map[string]any{"plant_id": pid}, tok)
	if resp.StatusCode != 400 {
		t.Fatalf("toggle -> %d", resp.StatusCode)
	}
	var detail map[string]any
	decode(t, resp, &detail)
	if !strings.Contains(detail["detail"].(string), "лампы") {
		t.Fatalf("detail %v", detail)
	}
	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	lightS, _ := sum["light"].(map[string]any)
	if lightS["lamp"] != nil {
		t.Fatalf("lamp %v", lightS["lamp"])
	}
}

func TestPauseOffLampIs409(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "pauseoff@example.com", "user-pass-1")
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "P"}, tok)
	decode(t, resp, &plant)
	var lamp map[string]any
	resp = req(t, "POST", "/api/lamps", map[string]any{"name": "L", "plant_ids": []int{int(plant["id"].(float64))}}, tok)
	decode(t, resp, &lamp)
	lampID := int(lamp["id"].(float64))
	resp = req(t, "POST", fmt.Sprintf("/api/lamps/%d/pause", lampID), nil, tok)
	if resp.StatusCode != 409 {
		t.Fatalf("pause off -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = req(t, "DELETE", fmt.Sprintf("/api/lamps/%d/pause", lampID), nil, tok)
	if resp.StatusCode != 409 {
		t.Fatalf("resume off -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------- авто-досветка (над DB, сеть не трогаем) ----------

func autoLampSetup(t *testing.T, day time.Time) (int, int, string, func(h, m int) time.Time) {
	admin := adminToken(t)
	uid, tok := makeUser(t, admin, fmt.Sprintf("autolamp%d@example.com", day.Unix()), "user-pass-1")
	var lemon map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Лимон", "light_target_hours": 12}, tok)
	decode(t, resp, &lemon)
	pid := int(lemon["id"].(float64))
	req(t, "PATCH", "/api/settings", map[string]any{"latitude": 55.75, "longitude": 37.62, "location_name": "Москва"}, tok)
	req(t, "PUT", "/api/settings/yandex-token", map[string]any{"token": "y0_test-token-123"}, tok)
	resp = req(t, "POST", "/api/lamps", map[string]any{"name": "Лампа цитрусы", "mode": "auto", "device_id": "dev-1", "device_name": "Розетка", "plant_ids": []int{pid}}, tok)
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create lamp -> %d: %s", resp.StatusCode, string(b))
	}
	var lamp map[string]any
	decode(t, resp, &lamp)
	lampID := int(lamp["id"].(float64))
	at := func(h, m int) time.Time { return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, zoneLoc) }
	if _, err := testPool.Exec(context.Background(), `INSERT INTO daylight_days (user_id, day, sunrise, sunset, daylight_hours, sunshine_hours)
		VALUES ($1, $2, $3, $4, 7.5, 2.0) ON CONFLICT (user_id, day) DO UPDATE SET sunrise=EXCLUDED.sunrise, sunset=EXCLUDED.sunset`,
		uid, day, at(9, 0), at(16, 30)); err != nil {
		t.Fatal(err)
	}
	return uid, lampID, tok, at
}

var zoneLoc, _ = time.LoadLocation("Europe/Moscow")

func autoSessions(t *testing.T, lampID int) [][3]any {
	rows, _ := testPool.Query(context.Background(), `SELECT source::text, started_at, ended_at FROM lamp_sessions WHERE lamp_id=$1 AND source='auto' ORDER BY started_at`, lampID)
	defer rows.Close()
	var out [][3]any
	for rows.Next() {
		var src string
		var s, e *time.Time
		rows.Scan(&src, &s, &e)
		out = append(out, [3]any{src, s, e})
	}
	return out
}

func TestAutoLampPlansAndDrivesPlug(t *testing.T) {
	day := time.Date(2030, 1, 15, 0, 0, 0, 0, zoneLoc)
	_, lampID, _, at := autoLampSetup(t, day)

	var calls []string
	testSrv.Lamps.YandexSetOn = func(token, dev string, on bool) error {
		if on {
			calls = append(calls, "on")
		} else {
			calls = append(calls, "off")
		}
		return nil
	}
	testSrv.Lamps.YandexListDevices = func(token string) ([]yandex.Device, error) { return nil, nil }
	light.FetchDays = func(lat, lon float64, tz *time.Location) ([]light.DayRow, error) { return nil, nil }

	_ = testSrv.Lamps.Tick(context.Background(), at(5, 0))
	plan := autoSessions(t, lampID)
	if len(plan) != 3 {
		t.Fatalf("plan %v", plan)
	}
	if !plan[0][1].(*time.Time).Equal(at(6, 0)) || !plan[0][2].(*time.Time).Equal(at(9, 0)) {
		t.Fatalf("morning %v", plan[0])
	}
	_ = testSrv.Lamps.Tick(context.Background(), at(6, 30))
	if len(calls) != 1 || calls[0] != "on" {
		t.Fatalf("calls %v", calls)
	}
	_ = testSrv.Lamps.Tick(context.Background(), at(6, 31))
	if len(calls) != 1 {
		t.Fatalf("repeat call %v", calls)
	}

	// выключили кнопкой посреди утра
	lamp, err := testSrv.Lamps.GetLamp(context.Background(), lampID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := testSrv.Lamps.Toggle(context.Background(), lamp, at(6, 40))
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	if calls[len(calls)-1] != "off" {
		t.Fatalf("toggle off %v", calls)
	}
	if err := testSrv.Lamps.ReplanAuto(context.Background(), lamp, at(6, 41)); err != nil {
		t.Fatalf("replan 6:41: %v", err)
	}
	if calls[len(calls)-1] != "off" {
		t.Fatalf("after toggle tick %v", calls)
	}
	plan = autoSessions(t, lampID)
	if len(plan) != 3 {
		t.Fatalf("plan after toggle %v", plan)
	}
	if !plan[1][2].(*time.Time).Equal(at(16, 30)) || !plan[1][1].(*time.Time).Before(at(16, 0)) {
		t.Fatalf("day part %v", plan[1])
	}

	_ = testSrv.Lamps.Tick(context.Background(), at(17, 0))
	if calls[len(calls)-1] != "on" {
		t.Fatalf("evening on %v", calls)
	}
	// смена режима во время вечерней досветки
	lamp, _ = testSrv.Lamps.GetLamp(context.Background(), lampID)
	_ = testSrv.Lamps.Update(context.Background(), lamp, map[string]any{"mode": "manual"}, nil, false, at(17, 5))
	plan = autoSessions(t, lampID)
	last := plan[len(plan)-1]
	if !last[2].(*time.Time).Equal(at(17, 5)) {
		t.Fatalf("last %v", last)
	}
	if calls[len(calls)-1] != "off" {
		t.Fatalf("update off %v", calls)
	}
}

func TestLocationFetchesDaylight(t *testing.T) {
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "sun@example.com", "user-pass-1")
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "P", "light_target_hours": 13}, tok)
	decode(t, resp, &plant)
	calls := 0
	light.FetchDays = func(lat, lon float64, tz *time.Location) ([]light.DayRow, error) {
		calls++
		zone := tz
		today := time.Now().In(zone)
		var rows []light.DayRow
		for i := -2; i <= 2; i++ {
			d := time.Date(today.Year(), today.Month(), today.Day()+i, 0, 0, 0, 0, time.UTC)
			sunrise := time.Date(today.Year(), today.Month(), today.Day()+i, 6, 0, 0, 0, zone)
			sunset := time.Date(today.Year(), today.Month(), today.Day()+i, 18, 0, 0, 0, zone)
			rows = append(rows, light.DayRow{Day: d, Sunrise: &sunrise, Sunset: &sunset, DaylightHours: 12.0, SunshineHours: 5.0})
		}
		return rows, nil
	}
	resp = req(t, "PATCH", "/api/settings", map[string]any{"location_name": "Мытищи", "latitude": 55.91, "longitude": 37.73}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("patch settings -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
	var today map[string]any
	getJSON(t, "/api/light/today", tok, &today)
	if today["sunshine_hours"] != float64(5.0) {
		t.Fatalf("today %v", today)
	}
	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", int(plant["id"].(float64))), tok, &sum)
	lightS, _ := sum["light"].(map[string]any)
	if lightS["natural_hours"] != float64(5.0) || lightS["deficit_hours"] != float64(8.0) {
		t.Fatalf("light %v", lightS)
	}
	// без лишнего запроса
	resp = req(t, "PATCH", "/api/settings", map[string]any{"location_name": "Мытищи", "latitude": 55.91, "longitude": 37.73}, tok)
	resp.Body.Close()
	if calls != 1 {
		t.Fatalf("extra calls %d", calls)
	}
}
