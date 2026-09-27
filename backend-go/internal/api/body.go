package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"poliv/internal/render"
	"poliv/internal/summary"
)

type body map[string]json.RawMessage

func readBody(r *http.Request) (body, error) {
	var m map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func (b body) has(key string) bool {
	_, ok := b[key]
	return ok
}

func (b body) string(key string) (*string, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (b body) int(key string) (*int, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (b body) float(key string) (*float64, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (b body) boolean(key string) (*bool, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// timeField — парсит RFC3339/ISO-строку (datetime) в time.Time.
func (b body) timeField(key string) (*time.Time, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// clockField — парсит "HH:MM[:SS]" в локальное время (date = today).
func (b body) clockField(key string, loc *time.Location) (time.Time, error) {
	var zero time.Time
	raw, ok := b[key]
	if !ok {
		return zero, nil
	}
	if string(raw) == "null" {
		return zero, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return zero, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return zero, nil
	}
	// Pydantic принимает HH:MM или HH:MM:SS
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return zero, errors.New("bad time")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return zero, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return zero, err
	}
	sec := 0
	if len(parts) == 3 {
		sec, _ = strconv.Atoi(parts[2])
	}
	now := time.Now().In(loc)
	y, mo, d := now.Date()
	return time.Date(y, mo, d, h, m, sec, 0, loc), nil
}

// dateField — парсит "YYYY-MM-DD" в summary.Date (UTC midnight).
func (b body) dateField(key string) (*summary.Date, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	d := summary.Date(t)
	return &d, nil
}

func (b body) intList(key string) ([]int, error) {
	raw, ok := b[key]
	if !ok {
		return nil, nil
	}
	var out []int
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// valueOrDefault nil-указатели в одинарные значения.
func fptr(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func renderF(f float64) render.F { return render.F(f) }
