package api

import (
	"net/http"
	"testing"
)

// TestHistoryInvalidTypeRejected — невалидный тип события в фильтре types → 422.
// Python-референс валидирует литерал и отвечает 422. Регрессия: Go молча принимал
// неизвестное значение и отвечал 200 с пустым результатом.
func TestHistoryInvalidTypeRejected(t *testing.T) {
	requireDB(t)
	tok := adminToken(t)
	if code := getJSON(t, "/api/plants/1/history?types=watering", tok, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("types=watering -> %d, ждём 422", code)
	}
	if code := getJSON(t, "/api/plants/1/history?types=water", tok, nil); code != 200 {
		t.Fatalf("types=water -> %d, ждём 200", code)
	}
}
