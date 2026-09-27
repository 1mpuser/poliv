// render — вывод значений так, как это делает Pydantic: числа float всегда с дробью (12.0),
// даты/время в ISO-формате с зоной, время суток как HH:MM:SS.
package render

import (
	"encoding/json"
	"strconv"
	"time"
)

// F — float64, сериализуется как Pydantic (Rust serde_json): целые → 12.0, дробные → 12.5.
type F float64

func (f F) MarshalJSON() ([]byte, error) {
	v := float64(f)
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !containsDotOrE(s) {
		s += ".0"
	}
	return []byte(s), nil
}

func containsDotOrE(s string) bool {
	for _, c := range s {
		if c == '.' || c == 'e' || c == 'E' {
			return true
		}
	}
	return false
}

// MarshalF прячет указатель на float в выводимый вид (для нулевых значений).
func MarshalF(v *float64) *F {
	if v == nil {
		return nil
	}
	f := F(*v)
	return &f
}

// Time — datetime в ISO-формате, как Python datetime.isoformat() в заданной зоне.
func Time(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	lt := t.In(loc)
	s := lt.Format("2006-01-02T15:04:05")
	micro := lt.Nanosecond() / 1000
	if micro != 0 {
		if micro%1000 == 0 {
			s += "." + pad3(micro/1000)
		} else {
			s += "." + pad6(micro)
		}
	}
	off := lt.Format("-07:00")
	if off == "+00:00" {
		off = "Z"
	}
	return s + off
}

// Date — дата как date.isoformat().
func Date(t time.Time) string {
	return t.Format("2006-01-02")
}

// ClockTime — время суток как time.isoformat() (HH:MM:SS).
func ClockTime(t time.Time) string {
	return t.Format("15:04:05")
}

func pad3(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func pad6(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

var _ = json.Marshal
