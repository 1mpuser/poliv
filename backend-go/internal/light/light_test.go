package light

import (
	"testing"
	"time"
)

// Open-Meteo отдаёт daylight_duration и sunshine_duration в секундах, в БД и сводке — часы.
func TestParseDaysConvertsSecondsToHours(t *testing.T) {
	tz, _ := time.LoadLocation("Europe/Moscow")
	data := map[string]any{"daily": map[string]any{
		"time":              []any{"2026-09-27"},
		"sunrise":           []any{"2026-09-27T06:47"},
		"sunset":            []any{"2026-09-27T18:32"},
		"daylight_duration": []any{42300.0},
		"sunshine_duration": []any{36402.9},
	}}
	rows, err := parseDays(data, tz)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if got := rows[0].DaylightHours; got != 11.75 {
		t.Errorf("DaylightHours = %v, want 11.75", got)
	}
	if got := rows[0].SunshineHours; got != 10.11 {
		t.Errorf("SunshineHours = %v, want 10.11", got)
	}
}

// Пустая длительность (null в ответе) — 0 часов, а не паника.
func TestParseDaysNullDuration(t *testing.T) {
	data := map[string]any{"daily": map[string]any{
		"time": []any{"2026-09-27"}, "sunrise": []any{nil}, "sunset": []any{nil},
		"daylight_duration": []any{nil}, "sunshine_duration": []any{nil},
	}}
	rows, err := parseDays(data, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].SunshineHours != 0 || rows[0].DaylightHours != 0 {
		t.Errorf("got %+v, want zero hours", rows[0])
	}
}
