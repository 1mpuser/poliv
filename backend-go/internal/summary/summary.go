// Правила статусов ухода. Чистые функции без БД — всё время приходит аргументами.
// Порт app/services/summary.py (см. backend/tests/test_summary.py, кейсы перенесены 1:1).
//
// Дни считаются календарными в локальном часовом поясе: полив вчера в 23:30
// и сегодня в 00:10 — это уже «1 день назад».
package summary

import (
	"math"
	"sort"
	"time"
)

// Date — календарная дата как time.Time в UTC на полночь (для арифметики дней).
type Date = time.Time

// Session — отрезок горения лампы; End == nil значит «горит сейчас».
type Session struct {
	Start time.Time
	End   *time.Time
}

// Period — период «растение стояло под лампой»; End == nil — стоит сейчас.
type Period struct {
	Start time.Time
	End   *time.Time
}

// Interval — интервал расписания (местное время суток).
type Interval struct {
	Start time.Time // часы:минуты (год/месяц/день не важны), в локальной зоне
	End   time.Time
}

const RepotSoonDays = 14
const EveningDefaultHour = 18

// ---------- утилиты ----------

// LocalDate возвращает календарную дату (в зоне loc) как UTC-полночь.
func LocalDate(dt time.Time, loc *time.Location) Date {
	y, m, d := dt.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// LocalMidnight — tz-осведомлённая полночь заданной даты в зоне loc.
func LocalMidnight(d Date, loc *time.Location) time.Time {
	y, m, dd := d.Year(), d.Month(), d.Day()
	return time.Date(y, m, dd, 0, 0, 0, 0, loc)
}

// Combine — tz-осведомлённый момент для даты и времени суток.
func Combine(d Date, hour, minute int, loc *time.Location) time.Time {
	y, m, dd := d.Year(), d.Month(), d.Day()
	return time.Date(y, m, dd, hour, minute, 0, 0, loc)
}

func DateAddDays(d Date, n int) Date {
	return d.AddDate(0, 0, n)
}

func DateDiff(a, b Date) int {
	return int(a.Sub(b).Hours() / 24)
}

// ---------- статус ----------

func StatusFor(dueIn, ahead int) string {
	if dueIn <= 0 {
		return "late"
	}
	if dueIn <= ahead {
		return "soon"
	}
	return "ok"
}

// ---------- полив / проверка грунта ----------

type WaterState struct {
	DaysSince       *int
	DaysSinceCheck  *int
	DueInDays       int
	Status          string
}

func WaterStateOf(lastWatered, lastChecked *time.Time, intervalDays, ahead int, now time.Time, loc *time.Location) WaterState {
	today := LocalDate(now, loc)
	var daysWater, daysCheck *int
	if lastWatered != nil {
		d := DateDiff(today, LocalDate(*lastWatered, loc))
		daysWater = &d
	}
	if lastChecked != nil {
		d := DateDiff(today, LocalDate(*lastChecked, loc))
		daysCheck = &d
	}
	if daysWater == nil && daysCheck == nil {
		return WaterState{nil, nil, 0, "late"}
	}
	touch := math.MaxInt32
	if daysWater != nil {
		touch = *daysWater
	}
	if daysCheck != nil && *daysCheck < touch {
		touch = *daysCheck
	}
	dueIn := intervalDays - touch
	return WaterState{daysWater, daysCheck, dueIn, StatusFor(dueIn, ahead)}
}

// ---------- подкормка ----------

type Fertilizer struct {
	ID                           int
	Name                         string
	NPK                          string
	RootDoseMlPerL               *float64
	FoliarDoseMlPerL             *float64
	IntervalDaysActiveSeason     int
	IntervalDaysDormantSeason    *int
}

func SeasonInterval(ft Fertilizer, season string) *int {
	if season == "dormant" {
		return ft.IntervalDaysDormantSeason
	}
	n := ft.IntervalDaysActiveSeason
	return &n
}

// PickNextFertilizer — следующий тип по кругу (порядок — по id), не совпадающий с последним.
func PickNextFertilizer(types []Fertilizer, lastTypeID *int) *Fertilizer {
	sorted := make([]Fertilizer, len(types))
	copy(sorted, types)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	if len(sorted) == 0 {
		return nil
	}
	if lastTypeID == nil {
		return &sorted[0]
	}
	for i := range sorted {
		if sorted[i].ID > *lastTypeID {
			return &sorted[i]
		}
	}
	return &sorted[0]
}

type FeedState struct {
	Next        *Fertilizer
	IntervalDays *int
	DueDate     *Date
	DueInDays   *int
	DaysSince   *int
	Status      string
}

func FeedStateOf(enabled bool, types []Fertilizer, lastFedAt *time.Time, lastTypeID *int, season string, ahead int, now time.Time, loc *time.Location) FeedState {
	today := LocalDate(now, loc)
	var daysSince *int
	if lastFedAt != nil {
		d := DateDiff(today, LocalDate(*lastFedAt, loc))
		daysSince = &d
	}
	nxt := PickNextFertilizer(types, lastTypeID)
	var interval *int
	if nxt != nil {
		interval = SeasonInterval(*nxt, season)
	}
	if !enabled || nxt == nil || interval == nil {
		return FeedState{nxt, interval, nil, nil, daysSince, "off"}
	}
	var dueDate Date
	if lastFedAt != nil {
		dueDate = DateAddDays(LocalDate(*lastFedAt, loc), *interval)
	} else {
		dueDate = today
	}
	dueIn := DateDiff(dueDate, today)
	return FeedState{nxt, interval, &dueDate, &dueIn, daysSince, StatusFor(dueIn, ahead)}
}

// ---------- лампа ----------

// LampHoursBetween — часы горения в окне [start, end). Пересечения объединяются, открытая сессия — до now.
func LampHoursBetween(sessions []Session, start, end, now time.Time) float64 {
	type seg struct{ s, e time.Time }
	var clipped []seg
	for _, s := range sessions {
		e := s.End
		if e == nil {
			e = &now
		}
		ss := s.Start
		if ss.Before(start) {
			ss = start
		}
		ee := *e
		if ee.After(end) {
			ee = end
		}
		if ee.After(now) {
			ee = now
		}
		if ee.After(ss) {
			clipped = append(clipped, seg{ss, ee})
		}
	}
	if len(clipped) == 0 {
		return 0
	}
	sort.Slice(clipped, func(i, j int) bool { return clipped[i].s.Before(clipped[j].s) })
	var total float64
	curS, curE := clipped[0].s, clipped[0].e
	for _, p := range clipped[1:] {
		if p.s.After(curE) {
			total += curE.Sub(curS).Hours()
			curS, curE = p.s, p.e
		} else if p.e.After(curE) {
			curE = p.e
		}
	}
	total += curE.Sub(curS).Hours()
	return total
}

// LampHoursInDay — часы лампы за день. plan=True — план на весь день.
func LampHoursInDay(sessions []Session, day Date, now time.Time, loc *time.Location, plan bool) float64 {
	start := LocalMidnight(day, loc)
	end := LocalMidnight(DateAddDays(day, 1), loc)
	if !plan {
		return LampHoursBetween(sessions, start, end, now)
	}
	closed := make([]Session, len(sessions))
	for i, s := range sessions {
		e := s.End
		if e == nil {
			nowCopy := now
			e = &nowCopy
		}
		closed[i] = Session{s.Start, e}
	}
	return LampHoursBetween(closed, start, end, end)
}

func LampHoursToday(sessions []Session, now time.Time, loc *time.Location) float64 {
	today := LocalDate(now, loc)
	return LampHoursBetween(sessions, LocalMidnight(today, loc), LocalMidnight(DateAddDays(today, 1), loc), now)
}

func LampStatus(hours, planned float64) string {
	if planned <= 0 {
		return "ok"
	}
	ratio := hours / planned
	if ratio >= 0.9 {
		return "ok"
	}
	if ratio >= 0.5 {
		return "soon"
	}
	return "late"
}

// ---------- свет ----------

type LightState struct {
	TotalHours   float64
	DeficitHours float64
	Status       string
}

func LightStateOf(target float64, natural *float64, lamp float64) LightState {
	var nat float64
	if natural != nil {
		nat = *natural
	}
	total := nat + lamp
	def := target - total
	if def < 0 {
		def = 0
	}
	return LightState{total, def, LampStatus(total, target)}
}

// ScheduleSessionsForDay — интервалы расписания → сессии лампы на конкретный день.
func ScheduleSessionsForDay(intervals []Interval, day Date, loc *time.Location) []Session {
	sorted := make([]Interval, len(intervals))
	copy(sorted, intervals)
	sort.Slice(sorted, func(i, j int) bool { return clockMinute(sorted[i].Start) < clockMinute(sorted[j].Start) })
	out := make([]Session, 0, len(sorted))
	for _, iv := range sorted {
		s := Combine(day, iv.Start.Hour(), iv.Start.Minute(), loc)
		e := Combine(day, iv.End.Hour(), iv.End.Minute(), loc)
		ee := e
		out = append(out, Session{s, &ee})
	}
	return out
}

func clockMinute(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

// SuggestLampWindow — когда добрать недостающий свет. Возвращает (start, end, до_полуночи) или nil.
type LampWindow struct {
	Start            time.Time
	End              time.Time
	UntilMidnight    bool
}

func SuggestLampWindow(deficit float64, sunset *time.Time, lampEnds []time.Time, day Date, loc *time.Location) *LampWindow {
	if deficit <= 0 {
		return nil
	}
	var starts []time.Time
	for _, e := range lampEnds {
		if LocalDate(e, loc).Equal(day) {
			starts = append(starts, e)
		}
	}
	if sunset != nil {
		starts = append(starts, *sunset)
	} else {
		starts = append(starts, Combine(day, EveningDefaultHour, 0, loc))
	}
	start := maxTime(starts)
	midnight := LocalMidnight(DateAddDays(day, 1), loc)
	end := start.Add(time.Duration(deficit * float64(time.Hour)))
	if end.After(midnight) {
		return &LampWindow{start, midnight, true}
	}
	return &LampWindow{start, end, false}
}

func maxTime(ts []time.Time) time.Time {
	m := ts[0]
	for _, t := range ts[1:] {
		if t.After(m) {
			m = t
		}
	}
	return m
}

// ---------- периоды привязки и досветка ----------

// ClipSession — части сессии лампы, пока растение стояло под ней.
func ClipSession(start time.Time, end *time.Time, periods []Period) []Session {
	var parts []Session
	for _, p := range periods {
		var s time.Time
		if start.After(p.Start) {
			s = start
		} else {
			s = p.Start
		}
		var e *time.Time
		if end == nil {
			e = p.End
		} else if p.End == nil {
			e = end
		} else {
			ee := minTime(*end, *p.End)
			c := ee
			e = &c
		}
		if e == nil || e.After(s) {
			parts = append(parts, Session{s, e})
		}
	}
	return parts
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func LampNeed(deficits []float64) float64 {
	m := 0.0
	for _, d := range deficits {
		if d > m {
			m = d
		}
	}
	return m
}

func PlanMorning(need float64, sunrise *time.Time, notBefore time.Time, day Date, loc *time.Location) *Session {
	if need <= 0 || sunrise == nil {
		return nil
	}
	start := sunrise.Add(-time.Duration(need / 2 * float64(time.Hour)))
	nb := Combine(day, notBefore.Hour(), notBefore.Minute(), loc)
	if start.Before(nb) {
		start = nb
	}
	if start.Before(*sunrise) {
		return &Session{start, sunrise}
	}
	return nil
}

func PlanEvening(remaining float64, sunset *time.Time, notAfter time.Time, day Date, loc *time.Location) *Session {
	if remaining <= 0 || sunset == nil {
		return nil
	}
	end := sunset.Add(time.Duration(remaining * float64(time.Hour)))
	na := Combine(day, notAfter.Hour(), notAfter.Minute(), loc)
	if end.After(na) {
		end = na
	}
	if end.After(*sunset) {
		return &Session{*sunset, &end}
	}
	return nil
}

func PlanDay(remaining float64, sunrise, sunset *time.Time) *Session {
	if remaining <= 0 || sunrise == nil || sunset == nil {
		return nil
	}
	start := sunset.Add(-time.Duration(remaining * float64(time.Hour)))
	if start.Before(*sunrise) {
		start = *sunrise
	}
	if start.Before(*sunset) {
		return &Session{start, sunset}
	}
	return nil
}

// ---------- пересадка ----------

func AddMonths(d Date, months int) Date {
	y, m, dd := d.Year(), d.Month(), d.Day()
	total := int(m) - 1 + months
	ny := y + total/12
	nm := time.Month(total%12 + 1)
	maxDay := daysInMonth(ny, nm)
	if dd > maxDay {
		dd = maxDay
	}
	return time.Date(ny, nm, dd, 0, 0, 0, 0, time.UTC)
}

func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

type RepotState struct {
	NextCheckDate Date
	DueInDays     int
	Status        string
}

func RepotStateOf(lastRepotted *time.Time, addedAt time.Time, months int, now time.Time, loc *time.Location) RepotState {
	var base Date
	if lastRepotted != nil {
		base = LocalDate(*lastRepotted, loc)
	} else {
		base = LocalDate(addedAt, loc)
	}
	nxt := AddMonths(base, months)
	dueIn := DateDiff(nxt, LocalDate(now, loc))
	return RepotState{nxt, dueIn, StatusFor(dueIn, RepotSoonDays)}
}

// ---------- статистика по неделям ----------

type WeekStats struct {
	WeekStart      Date
	Waterings      int
	Feedings       int
	LampHours      float64
	IsCurrent      bool
	SunshineHours  float64
}

func WeekOf(dt time.Time, loc *time.Location) Date {
	d := LocalDate(dt, loc)
	// понедельник = weekday(): 0 в Go (воскресенье), в Python 0 — понедельник
	wd := int(d.Weekday()) // 0=Sunday..6=Saturday
	// Python weekday(): 0=Monday..6=Sunday. Сдвиг: (wd+6)%7
	offset := (wd + 6) % 7
	return DateAddDays(d, -offset)
}

func WeeklyStats(waterings, feedings []time.Time, lampSessions []Session, weeks int, now time.Time, loc *time.Location, sunshine map[Date]float64) []WeekStats {
	current := WeekOf(now, loc)
	starts := make([]Date, weeks)
	for i := 0; i < weeks; i++ {
		starts[i] = current.AddDate(0, 0, -7*(weeks-1-i))
	}
	wCount := map[Date]int{}
	for _, dt := range waterings {
		wCount[WeekOf(dt, loc)]++
	}
	fCount := map[Date]int{}
	for _, dt := range feedings {
		fCount[WeekOf(dt, loc)]++
	}
	out := make([]WeekStats, 0, weeks)
	for _, ws := range starts {
		winS := LocalMidnight(ws, loc)
		winE := LocalMidnight(DateAddDays(ws, 7), loc)
		sun := 0.0
		for d, h := range sunshine {
			if !d.Before(ws) && d.Before(DateAddDays(ws, 7)) {
				sun += h
			}
		}
		out = append(out, WeekStats{
			WeekStart:     ws,
			Waterings:     wCount[ws],
			Feedings:      fCount[ws],
			LampHours:     LampHoursBetween(lampSessions, winS, winE, now),
			IsCurrent:     ws.Equal(current),
			SunshineHours: sun,
		})
	}
	return out
}
