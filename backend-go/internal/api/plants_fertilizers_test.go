package api

import (
	"fmt"
	"testing"
)

// TestPlantFertilizerBinding — у растения свой набор удобрений и свой интервал подкормки:
// они отдаются в ответе и используются в сводке (следующее удобрение — из набора, интервал растения).
func TestPlantFertilizerBinding(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "bind@example.com", "user-pass-1")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid0 := int(ferts[0]["id"].(float64))
	fid1 := int(ferts[1]["id"].(float64))

	// растение только с первым удобрением
	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Фикус", "fertilizer_ids": []int{fid0}}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create -> %d", resp.StatusCode)
	}
	decode(t, resp, &plant)
	pid := int(plant["id"].(float64))
	ids := plant["fertilizer_ids"].([]any)
	if len(ids) != 1 || int(ids[0].(float64)) != fid0 {
		t.Fatalf("fertilizer_ids %v", ids)
	}

	// PATCH: добавить второе удобрение и поставить свой интервал 7
	resp = req(t, "PATCH", fmt.Sprintf("/api/plants/%d", pid),
		map[string]any{"fertilizer_ids": []int{fid0, fid1}, "fertilizing_interval_days": 7}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("patch -> %d", resp.StatusCode)
	}
	decode(t, resp, &plant)
	ids = plant["fertilizer_ids"].([]any)
	if len(ids) != 2 {
		t.Fatalf("after patch ids %v", ids)
	}
	if plant["fertilizing_interval_days"].(float64) != 7 {
		t.Fatalf("interval %v", plant["fertilizing_interval_days"])
	}

	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	fs := sum["fertilizers"].([]any)
	if len(fs) != 2 || int(fs[0].(map[string]any)["id"].(float64)) != fid0 {
		t.Fatalf("summary fertilizers %v", fs)
	}
	feed := sum["feed"].(map[string]any)
	if feed["interval_days"] == nil || int(feed["interval_days"].(float64)) != 7 {
		t.Fatalf("feed interval %v", feed["interval_days"])
	}
	next := feed["next"].(map[string]any)
	if int(next["id"].(float64)) != fid0 {
		t.Fatalf("next %v", next)
	}
}

// TestFeedingFertilizerNotInPlantSet422 — подкормка растения удобрением не из его набора → 422,
// своё (из набора) — создаётся.
func TestFeedingFertilizerNotInPlantSet422(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "set422@example.com", "user-pass-1")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid0 := int(ferts[0]["id"].(float64))
	fid1 := int(ferts[1]["id"].(float64))

	pid := createPlantFor(t, tok, "Фикус")
	// оставляем только fid0
	resp := req(t, "PATCH", fmt.Sprintf("/api/plants/%d", pid), map[string]any{"fertilizer_ids": []int{fid0}}, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("patch -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	// чужое для растения удобрение (но своё для учётки) → 422
	resp = req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid1, "method": "root"}, tok)
	if resp.StatusCode != 422 {
		t.Fatalf("feed fid1 -> %d, ждём 422", resp.StatusCode)
	}
	var detail map[string]any
	decode(t, resp, &detail)
	if msg, ok := detail["detail"].(string); !ok || msg == "" {
		t.Fatalf("detail %v", detail)
	}
	resp.Body.Close()

	// своё для растения → 201
	resp = req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid0, "method": "root"}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("feed fid0 -> %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestFirstFeedingSetsNext — после подкормки следующим становится следующее по кругу из набора растения.
func TestFirstFeedingSetsNext(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "nextset@example.com", "user-pass-1")

	var ferts []map[string]any
	getJSON(t, "/api/fertilizers", tok, &ferts)
	fid1 := int(ferts[1]["id"].(float64))

	pid := createPlantFor(t, tok, "Фикус")
	resp := req(t, "POST", "/api/feedings", map[string]any{"plant_id": pid, "fertilizer_type_id": fid1, "method": "root"}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("feed -> %d", resp.StatusCode)
	}
	resp.Body.Close()

	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	feed := sum["feed"].(map[string]any)
	next := feed["next"].(map[string]any)
	if int(next["id"].(float64)) == fid1 {
		t.Fatalf("next должен отличаться от последней подкормки: %v", next)
	}
}

// TestPlantFertilizerIsolation — чужие удобрения в наборе растения не попадают: 404,
// и подкормка чужим удобрением своего растения тоже 404.
func TestPlantFertilizerIsolation(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, a := makeUser(t, admin, "iso-a@example.com", "user-pass-1")
	_, b := makeUser(t, admin, "iso-b@example.com", "user-pass-1")

	var fertsA []map[string]any
	getJSON(t, "/api/fertilizers", a, &fertsA)
	fidA := int(fertsA[0]["id"].(float64))

	// b не может создать растение с удобрением a
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Фикус B", "fertilizer_ids": []int{fidA}}, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign create -> %d, ждём 404", resp.StatusCode)
	}
	resp.Body.Close()

	// b не может добавить удобрение a через PATCH
	var plantB map[string]any
	resp = req(t, "POST", "/api/plants", map[string]any{"name": "Фикус B"}, b)
	if resp.StatusCode != 201 {
		t.Fatalf("create b -> %d", resp.StatusCode)
	}
	decode(t, resp, &plantB)
	pidB := int(plantB["id"].(float64))
	resp = req(t, "PATCH", fmt.Sprintf("/api/plants/%d", pidB), map[string]any{"fertilizer_ids": []int{fidA}}, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign patch -> %d, ждём 404", resp.StatusCode)
	}
	resp.Body.Close()

	// b не может подкормить своё растение чужим удобрением
	resp = req(t, "POST", "/api/feedings", map[string]any{"plant_id": pidB, "fertilizer_type_id": fidA, "method": "root"}, b)
	if resp.StatusCode != 404 {
		t.Fatalf("foreign feed -> %d, ждём 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestPlantNoFertilizersNotLate — растение без привязанных удобрений: подкормка выключена,
// не «просрочена», следующего удобрения нет.
func TestPlantNoFertilizersNotLate(t *testing.T) {
	requireDB(t)
	admin := adminToken(t)
	_, tok := makeUser(t, admin, "nofert@example.com", "user-pass-1")

	var plant map[string]any
	resp := req(t, "POST", "/api/plants", map[string]any{"name": "Фикус"}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create -> %d", resp.StatusCode)
	}
	decode(t, resp, &plant)
	pid := int(plant["id"].(float64))

	var sum map[string]any
	getJSON(t, fmt.Sprintf("/api/plants/%d/summary", pid), tok, &sum)
	feed := sum["feed"].(map[string]any)
	if feed["status"] != "off" {
		t.Fatalf("status %v", feed["status"])
	}
	if feed["next"] != nil || feed["due_in_days"] != nil {
		t.Fatalf("feed %v", feed)
	}
}
