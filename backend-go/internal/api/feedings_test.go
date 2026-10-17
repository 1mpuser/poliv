package api

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestRootFeedingCreatesAutoWatering — корневая подкормка автоматически создаёт полив:
// статус полива сбрасывается, days_since = 0, в истории две записи, полив один.
func TestRootFeedingCreatesAutoWatering(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "rootfeed@example.com", "user-pass-1")
	pid := createPlantFor(t, tok, "Фикус")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid := int(ferts[0]["id"].(float64))

	resp := req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid, "method": "root"}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create feeding -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	water, _ := sum["water"].(map[string]any)
	if water["status"] != "ok" || water["days_since"] != float64(0) {
		t.Fatalf("water summary %v", water)
	}

	var hist []map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/history?types=water&types=feed", pid), tok, &hist)
	if len(hist) != 2 {
		t.Fatalf("history %v", hist)
	}

	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings %d", n)
	}
}

// TestFoliarFeedingNoAutoWatering — листовая (некорневая) подкормка полив не создаёт.
func TestFoliarFeedingNoAutoWatering(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "foliarfeed@example.com", "user-pass-1")
	pid := createPlantFor(t, tok, "Фикус")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid := int(ferts[0]["id"].(float64))

	resp := req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid, "method": "foliar"}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create feeding -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	if n := waterCount(t, pid); n != 0 {
		t.Fatalf("waterings %d", n)
	}
}

// TestRootFeedingNoDuplicateSameDay — если полив в тот же день уже есть, дубль не создаётся,
// и удаление подкормки не трогает чужой (вручную созданный) полив.
func TestRootFeedingNoDuplicateSameDay(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "dupday@example.com", "user-pass-1")
	pid := createPlantFor(t, tok, "Фикус")

	// одно и то же время — чтобы тест не зависел от границы суток
	at := time.Now().In(zoneLoc).Format(time.RFC3339)
	resp := req(t, "POST", "/api/waterings", map[string]any{"plant_id": pid, "watered_at": at}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create watering -> %d", resp.StatusCode)
	}
	decode(t, resp, nil)

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid := int(ferts[0]["id"].(float64))

	var feed map[string]any
	resp = req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid, "method": "root", "fed_at": at}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create feeding -> %d", resp.StatusCode)
	}
	decode(t, resp, &feed)
	fid2 := int(feed["id"].(float64))

	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings %d, дубль создан", n)
	}
	// у такой подкормки ссылки на автополив нет
	var wateringID *int
	_ = testPool.QueryRow(context.Background(), `SELECT watering_id FROM feeding_logs WHERE id=$1`, fid2).Scan(&wateringID)
	if wateringID != nil {
		t.Fatalf("watering_id %v, дубль привязан", wateringID)
	}

	// удаление подкормки не удаляет вручную созданный полив
	resp = req(t, "DELETE", fmt.Sprintf("/api/feedings/%d", fid2), nil, tok)
	if resp.StatusCode != 204 {
		t.Fatalf("delete feeding -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings after delete %d", n)
	}
}

// TestDeleteFeedingRemovesAutoWatering — удаление корневой подкормки убирает и автополив.
func TestDeleteFeedingRemovesAutoWatering(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "delfeed@example.com", "user-pass-1")
	pid := createPlantFor(t, tok, "Фикус")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid := int(ferts[0]["id"].(float64))

	var feed map[string]any
	resp := req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid, "method": "root"}, tok)
	decode(t, resp, &feed)
	feedID := int(feed["id"].(float64))

	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings %d", n)
	}

	resp = req(t, "DELETE", fmt.Sprintf("/api/feedings/%d", feedID), nil, tok)
	if resp.StatusCode != 204 {
		t.Fatalf("delete feeding -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	if n := waterCount(t, pid); n != 0 {
		t.Fatalf("waterings after delete %d", n)
	}
}

// TestUpdateFeedingReconcilesAutoWatering — смена метода/времени держит автополив в согласии:
// root→foliar удаляет, foliar→root создаёт, время двигается вместе.
func TestUpdateFeedingReconcilesAutoWatering(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "updfeed@example.com", "user-pass-1")
	pid := createPlantFor(t, tok, "Фикус")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid := int(ferts[0]["id"].(float64))

	var feed map[string]any
	resp := req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid, "method": "root"}, tok)
	decode(t, resp, &feed)
	feedID := int(feed["id"].(float64))

	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings %d", n)
	}

	// root -> foliar: автополив удаляется
	resp = req(t, "PATCH", fmt.Sprintf("/api/feedings/%d", feedID), map[string]any{"method": "foliar"}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("patch foliar -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	if n := waterCount(t, pid); n != 0 {
		t.Fatalf("waterings after foliar %d", n)
	}

	// foliar -> root: автополив создаётся снова
	resp = req(t, "PATCH", fmt.Sprintf("/api/feedings/%d", feedID), map[string]any{"method": "root"}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("patch root -> %d", resp.StatusCode)
	}
	resp.Body.Close()
	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings after root %d", n)
	}
}

// TestFeedingForeign404 — чужую подкормку не увидеть и не удалить (изоляция по учёткам).
func TestFeedingForeign404(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, a := makeUser(t, admin, "fa@example.com", "user-pass-1")
	_, b := makeUser(t, admin, "fb@example.com", "user-pass-1")

	pid := createPlantFor(t, a, "Фикус A")
	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", a, &ferts)
	fid := int(ferts[0]["id"].(float64))

	var feed map[string]any
	resp := req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid, "method": "root"}, a)
	decode(t, resp, &feed)
	feedID := int(feed["id"].(float64))

	resp = req(t, "DELETE", fmt.Sprintf("/api/feedings/%d", feedID), nil, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign delete -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = req(t, "PATCH", fmt.Sprintf("/api/feedings/%d", feedID), map[string]any{"method": "foliar"}, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign patch -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	// чужая корневая подкормка полив в чужой учётке не создаёт
	if n := waterCount(t, pid); n != 1 {
		t.Fatalf("waterings %d", n)
	}
}

func createPlantFor(t *testing.T, token, name string) int {
	t.Helper()
	// привязываем все удобрения учётки — как у существующих растений после бэкфилла
	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", token, &ferts)
	ids := make([]int, 0, len(ferts))
	for _, f := range ferts {
		ids = append(ids, int(f["id"].(float64)))
	}
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": name, "fertilizer_ids": ids}, token)
	if resp.StatusCode != 201 {
		t.Fatalf("create plant -> %d", resp.StatusCode)
	}
	decode(t, resp, &plant)
	return int(plant["id"].(float64))
}

func waterCount(t *testing.T, plantID int) int {
	t.Helper()
	var cnt int
	_ = testPool.QueryRow(context.Background(), `SELECT count(*) FROM watering_logs WHERE plant_id=$1`, plantID).Scan(&cnt)
	return cnt
}
