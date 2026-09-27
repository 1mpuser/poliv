package summary_test

import (
	"math"
	"testing"
	"time"

	"poliv/internal/summary"
)

var mskLoc = mustLoc("Europe/Moscow")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

func msk(parts ...int) time.Time {
	p := make([]int, 7)
	copy(p, parts)
	_ = p
	return time.Date(parts[0], time.Month(parts[1]), parts[2], at(parts, 3), at(parts, 4), at(parts, 5), at(parts, 6), mskLoc)
}

func at(p []int, i int) int {
	if i < len(p) {
		return p[i]
	}
	return 0
}

func mkdate(y, mo, d int) summary.Date {
	return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
}

func ft(id, active, dormant int) summary.Fertilizer {
	var drm *int
	if dormant > 0 {
		d := dormant
		drm = &d
	}
	return summary.Fertilizer{ID: id, IntervalDaysActiveSeason: active, IntervalDaysDormantSeason: drm}
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func tpi(n int) *int { return &n }

func fp(f float64) *float64 { return &f }

func endEq(e *time.Time, tm time.Time) bool {
	return e != nil && e.Equal(tm)
}

func tp(ts ...time.Time) *time.Time {
	if len(ts) == 0 {
		return nil
	}
	return &ts[0]
}

func makeSessions(now time.Time, pairs ...[2]time.Time) []summary.Session {
	var out []summary.Session
	for _, p := range pairs {
		e := p[1]
		var ep *time.Time
		if !e.IsZero() {
			ep = &e
		}
		out = append(out, summary.Session{Start: p[0], End: ep})
	}
	return out
}

func TestStatusFor(t *testing.T) {
	cases := []struct{ due, ahead int; want string }{
		{5, 1, "ok"}, {1, 1, "soon"}, {2, 2, "soon"}, {0, 1, "late"}, {-3, 1, "late"}, {1, 0, "ok"},
	}
	for _, c := range cases {
		if got := summary.StatusFor(c.due, c.ahead); got != c.want {
			t.Errorf("StatusFor(%d,%d)=%q want %q", c.due, c.ahead, got, c.want)
		}
	}
}

func TestWaterCountsCalendarDays(t *testing.T) {
	s := summary.WaterStateOf(tp(msk(2026, 9, 22, 23, 30)), nil, 4, 1, msk(2026, 9, 23, 0, 10), mskLoc)
	if s.DaysSince == nil || *s.DaysSince != 1 || s.DueInDays != 3 || s.Status != "ok" {
		t.Fatalf("got %+v", s)
	}
}

func TestWaterSoonAndLate(t *testing.T) {
	now := msk(2026, 9, 23, 12)
	if summary.WaterStateOf(tp(msk(2026, 9, 20, 9)), nil, 4, 1, now, mskLoc).Status != "soon" {
		t.Fatal("expected soon")
	}
	if summary.WaterStateOf(tp(msk(2026, 9, 19, 9)), nil, 4, 1, now, mskLoc).Status != "late" {
		t.Fatal("expected late")
	}
}

func TestWaterNeverWateredIsLate(t *testing.T) {
	s := summary.WaterStateOf(nil, nil, 4, 1, msk(2026, 9, 23, 12), mskLoc)
	if s.DaysSince != nil || s.DaysSinceCheck != nil || s.Status != "late" {
		t.Fatalf("got %+v", s)
	}
}

func TestSoilCheckResetsCounter(t *testing.T) {
	s := summary.WaterStateOf(tp(msk(2026, 9, 20, 9)), tp(msk(2026, 9, 22, 9)), 4, 1, msk(2026, 9, 23, 12), mskLoc)
	if s.DaysSince == nil || *s.DaysSince != 3 || s.DaysSinceCheck == nil || *s.DaysSinceCheck != 1 {
		t.Fatalf("days got %+v", s)
	}
	if s.DueInDays != 3 || s.Status != "ok" {
		t.Fatalf("got %+v", s)
	}
}

func TestWateringAlsoResetsCounter(t *testing.T) {
	s := summary.WaterStateOf(tp(msk(2026, 9, 22, 9)), tp(msk(2026, 9, 20, 9)), 4, 1, msk(2026, 9, 23, 12), mskLoc)
	if s.DaysSince == nil || *s.DaysSince != 1 || s.DaysSinceCheck == nil || *s.DaysSinceCheck != 3 {
		t.Fatalf("days got %+v", s)
	}
	if s.Status != "ok" {
		t.Fatal("expected ok")
	}
}

func TestLatestTouchWinsEvenIfEarlyCheck(t *testing.T) {
	s := summary.WaterStateOf(tp(msk(2026, 9, 18, 9)), tp(msk(2026, 9, 22, 9)), 4, 1, msk(2026, 9, 23, 12), mskLoc)
	if s.DaysSince == nil || *s.DaysSince != 5 || s.DueInDays != 3 || s.Status != "ok" {
		t.Fatalf("got %+v", s)
	}
}

func TestNextFertilizer(t *testing.T) {
	lomo := ft(1, 14, 30)
	bona := ft(2, 10, 0)
	third := ft(3, 7, 21)
	if got := summary.PickNextFertilizer([]summary.Fertilizer{bona, lomo}, nil); got.ID != 1 {
		t.Fatal("first when no history")
	}
	if got := summary.PickNextFertilizer([]summary.Fertilizer{lomo, bona}, tpi(1)); got.ID != 2 {
		t.Fatal("alternate 1")
	}
	if got := summary.PickNextFertilizer([]summary.Fertilizer{lomo, bona}, tpi(2)); got.ID != 1 {
		t.Fatal("alternate 2")
	}
	types := []summary.Fertilizer{lomo, bona, third}
	if got := summary.PickNextFertilizer(types, tpi(2)); got.ID != 3 {
		t.Fatal("cycle to third")
	}
	if got := summary.PickNextFertilizer(types, tpi(3)); got.ID != 1 {
		t.Fatal("cycle to first")
	}
	if got := summary.PickNextFertilizer([]summary.Fertilizer{lomo}, tpi(1)); got.ID != 1 {
		t.Fatal("single repeats")
	}
	if got := summary.PickNextFertilizer(nil, nil); got != nil {
		t.Fatal("empty -> nil")
	}
	// последний тип (id=2) удалён — берём следующий по id
	if got := summary.PickNextFertilizer([]summary.Fertilizer{lomo, third}, tpi(2)); got.ID != 3 {
		t.Fatal("after deleted type")
	}
}

func TestFeedDueUsesNextTypeInterval(t *testing.T) {
	lomo := ft(1, 14, 30)
	bona := ft(2, 10, 0)
	now := msk(2026, 9, 23, 12)
	s := summary.FeedStateOf(true, []summary.Fertilizer{lomo, bona}, tp(msk(2026, 9, 15, 9)), tpi(1), "active", 1, now, mskLoc)
	if s.Next == nil || s.Next.ID != 2 || s.IntervalDays == nil || *s.IntervalDays != 10 {
		t.Fatalf("got %+v", s)
	}
	if s.DueDate == nil || !s.DueDate.Equal(mkdate(2026, 9, 25)) {
		t.Fatalf("due date %v", s.DueDate)
	}
	if s.DueInDays == nil || *s.DueInDays != 2 || s.Status != "ok" || s.DaysSince == nil || *s.DaysSince != 8 {
		t.Fatalf("got %+v", s)
	}
}

func TestFeedDormantWithoutIntervalIsOff(t *testing.T) {
	lomo := ft(1, 14, 30)
	bona := ft(2, 10, 0)
	s := summary.FeedStateOf(true, []summary.Fertilizer{lomo, bona}, tp(msk(2026, 9, 15)), tpi(1), "dormant", 1, msk(2026, 9, 23), mskLoc)
	if s.Next == nil || s.Next.ID != 2 || s.Status != "off" || s.DueDate != nil {
		t.Fatalf("got %+v", s)
	}
}

func TestFeedDisabledIsOff(t *testing.T) {
	lomo := ft(1, 14, 30)
	bona := ft(2, 10, 0)
	s := summary.FeedStateOf(false, []summary.Fertilizer{lomo, bona}, nil, nil, "active", 1, msk(2026, 9, 23), mskLoc)
	if s.Status != "off" || s.Next == nil || s.Next.ID != 1 {
		t.Fatalf("got %+v", s)
	}
}

func TestFeedNeverFedIsDueToday(t *testing.T) {
	lomo := ft(1, 14, 30)
	s := summary.FeedStateOf(true, []summary.Fertilizer{lomo}, nil, nil, "active", 1, msk(2026, 9, 23), mskLoc)
	if s.DueInDays == nil || *s.DueInDays != 0 || s.Status != "late" {
		t.Fatalf("got %+v", s)
	}
}

func TestLampTodayClipsAndCountsOpen(t *testing.T) {
	now := msk(2026, 9, 23, 10)
	sessions := []summary.Session{
		{Start: msk(2026, 9, 22, 20), End: tp(msk(2026, 9, 23, 2))},
		{Start: msk(2026, 9, 23, 7), End: nil},
	}
	if got := summary.LampHoursToday(sessions, now, mskLoc); !approx(got, 5) {
		t.Fatalf("got %v", got)
	}
}

func TestLampOverlappingNotDoubleCounted(t *testing.T) {
	sessions := []summary.Session{
		{Start: msk(2026, 9, 23, 8), End: tp(msk(2026, 9, 23, 12))},
		{Start: msk(2026, 9, 23, 10), End: tp(msk(2026, 9, 23, 14))},
	}
	if got := summary.LampHoursToday(sessions, msk(2026, 9, 23, 20), mskLoc); !approx(got, 6) {
		t.Fatalf("got %v", got)
	}
}

func TestLampHoursBetweenWindow(t *testing.T) {
	sessions := []summary.Session{{Start: msk(2026, 9, 21, 22), End: tp(msk(2026, 9, 22, 3))}}
	if got := summary.LampHoursBetween(sessions, msk(2026, 9, 22), msk(2026, 9, 23), msk(2026, 9, 30)); !approx(got, 3) {
		t.Fatalf("got %v", got)
	}
}

func TestLampStatus(t *testing.T) {
	cases := []struct{ hours, planned float64; want string }{
		{12, 12, "ok"}, {11, 12, "ok"}, {7, 12, "soon"}, {5, 12, "late"}, {0, 0, "ok"},
	}
	for _, c := range cases {
		if got := summary.LampStatus(c.hours, c.planned); got != c.want {
			t.Errorf("LampStatus(%v,%v)=%q want %q", c.hours, c.planned, got, c.want)
		}
	}
}

func TestAddMonthsClampsDay(t *testing.T) {
	if got := summary.AddMonths(mkdate(2026, 1, 31), 1); !got.Equal(mkdate(2026, 2, 28)) {
		t.Fatalf("got %v", got)
	}
	if got := summary.AddMonths(mkdate(2026, 11, 15), 3); !got.Equal(mkdate(2027, 2, 15)) {
		t.Fatalf("got %v", got)
	}
}

func TestRepotFromLastRepot(t *testing.T) {
	s := summary.RepotStateOf(tp(msk(2026, 9, 2, 18)), msk(2025, 1, 1), 6, msk(2026, 9, 23), mskLoc)
	if !s.NextCheckDate.Equal(mkdate(2027, 3, 2)) || s.Status != "ok" {
		t.Fatalf("got %+v", s)
	}
}

func TestRepotFallsBackAndSoon(t *testing.T) {
	s := summary.RepotStateOf(nil, msk(2025, 10, 1), 12, msk(2026, 9, 23), mskLoc)
	if !s.NextCheckDate.Equal(mkdate(2026, 10, 1)) || s.DueInDays != 8 || s.Status != "soon" {
		t.Fatalf("got %+v", s)
	}
}

func TestWeeklyStatsMonday(t *testing.T) {
	now := msk(2026, 9, 23, 12)
	water := []time.Time{msk(2026, 9, 21, 9), msk(2026, 9, 20, 9), msk(2026, 9, 14, 9)}
	feed := []time.Time{msk(2026, 9, 22, 9)}
	sess := []summary.Session{{Start: msk(2026, 9, 20, 20), End: tp(msk(2026, 9, 21, 2))}}
	st := summary.WeeklyStats(water, feed, sess, 2, now, mskLoc, nil)
	if !st[0].WeekStart.Equal(mkdate(2026, 9, 14)) || !st[1].WeekStart.Equal(mkdate(2026, 9, 21)) {
		t.Fatalf("starts %v %v", st[0].WeekStart, st[1].WeekStart)
	}
	if st[0].Waterings != 2 || st[1].Waterings != 1 {
		t.Fatalf("waterings %v", st)
	}
	if st[0].Feedings != 0 || st[1].Feedings != 1 {
		t.Fatalf("feedings %v", st)
	}
	if !approx(st[0].LampHours, 4) || !approx(st[1].LampHours, 2) {
		t.Fatalf("lamphours %v", st)
	}
	if !st[1].IsCurrent || st[0].IsCurrent {
		t.Fatal("is_current wrong")
	}
}

func TestLampOpenSessionDoesNotCountFuture(t *testing.T) {
	now := msk(2026, 9, 23, 10)
	s := []summary.Session{{Start: now.Add(-time.Hour), End: nil}}
	if got := summary.LampHoursToday(s, now, mskLoc); !approx(got, 1) {
		t.Fatalf("got %v", got)
	}
}

func TestScheduleSessionsForDay(t *testing.T) {
	day := mkdate(2026, 9, 24)
	ivs := []summary.Interval{{Start: msk(2026, 9, 24, 17), End: msk(2026, 9, 24, 21, 30)}, {Start: msk(2026, 9, 24, 7), End: msk(2026, 9, 24, 10)}}
	s := summary.ScheduleSessionsForDay(ivs, day, mskLoc)
	want1 := msk(2026, 9, 24, 7)
	want1e := msk(2026, 9, 24, 10)
	want2 := msk(2026, 9, 24, 17)
	want2e := msk(2026, 9, 24, 21, 30)
	if !s[0].Start.Equal(want1) || !endEq(s[0].End, want1e) || !s[1].Start.Equal(want2) || !endEq(s[1].End, want2e) {
		t.Fatalf("got %+v", s)
	}
}

func TestLampHoursInDayPlan(t *testing.T) {
	now := msk(2026, 9, 24, 12)
	day := mkdate(2026, 9, 24)
	sessions := []summary.Session{
		{Start: msk(2026, 9, 24, 17), End: tp(msk(2026, 9, 24, 21))},
		{Start: msk(2026, 9, 24, 10), End: nil},
	}
	if got := summary.LampHoursInDay(sessions, day, now, mskLoc, true); !approx(got, 6) {
		t.Fatalf("plan true got %v", got)
	}
	if got := summary.LampHoursInDay(sessions, day, now, mskLoc, false); !approx(got, 2) {
		t.Fatalf("plan false got %v", got)
	}
}

func TestLightState(t *testing.T) {
	cases := []struct {
		nat  *float64
		lamp float64
		tar  float64
		tot  float64
		def  float64
		st   string
	}{
		{fp(1.5), 7, 13, 8.5, 4.5, "soon"},
		{fp(10), 4, 13, 14, 0, "ok"},
		{fp(0), 3, 13, 3, 10, "late"},
		{nil, 12, 12, 12, 0, "ok"},
	}
	for _, c := range cases {
		st := summary.LightStateOf(c.tar, c.nat, c.lamp)
		if !approx(st.TotalHours, c.tot) || !approx(st.DeficitHours, c.def) || st.Status != c.st {
			t.Fatalf("got %+v", st)
		}
	}
}

func TestSuggestWindowStartsAfterSunsetAndLastLamp(t *testing.T) {
	day := mkdate(2026, 9, 24)
	sunset := msk(2026, 9, 24, 18, 23)
	w := summary.SuggestLampWindow(2.5, &sunset, []time.Time{msk(2026, 9, 24, 19)}, day, mskLoc)
	if w == nil || !w.Start.Equal(msk(2026, 9, 24, 19)) || !w.End.Equal(msk(2026, 9, 24, 21, 30)) || w.UntilMidnight {
		t.Fatalf("got %+v", w)
	}
}

func TestSuggestWindowWithoutLocation(t *testing.T) {
	w := summary.SuggestLampWindow(3, nil, nil, mkdate(2026, 9, 24), mskLoc)
	if w == nil || !w.Start.Equal(msk(2026, 9, 24, 18)) || !w.End.Equal(msk(2026, 9, 24, 21)) || w.UntilMidnight {
		t.Fatalf("got %+v", w)
	}
}

func TestSuggestWindowClampedAtMidnight(t *testing.T) {
	sunset := msk(2026, 9, 24, 18)
	w := summary.SuggestLampWindow(8, &sunset, nil, mkdate(2026, 9, 24), mskLoc)
	if w == nil || !w.End.Equal(msk(2026, 9, 25, 0)) || !w.UntilMidnight {
		t.Fatalf("got %+v", w)
	}
}

func TestSuggestWindowNoneWhenEnough(t *testing.T) {
	sunset := msk(2026, 9, 24, 18)
	if summary.SuggestLampWindow(0, &sunset, nil, mkdate(2026, 9, 24), mskLoc) != nil {
		t.Fatal("expected nil")
	}
}

func TestWeeklyStatsSunshine(t *testing.T) {
	sun := map[summary.Date]float64{mkdate(2026, 9, 21): 5.5, mkdate(2026, 9, 22): 1.0}
	st := summary.WeeklyStats(nil, nil, nil, 1, msk(2026, 9, 23, 12), mskLoc, sun)
	if !approx(st[0].SunshineHours, 6.5) {
		t.Fatalf("got %v", st[0].SunshineHours)
	}
}

func TestClipSessionToPeriods(t *testing.T) {
	periods := []summary.Period{
		{Start: msk(2026, 1, 1, 8), End: tp(msk(2026, 1, 1, 10))},
		{Start: msk(2026, 1, 1, 12), End: nil},
	}
	res := summary.ClipSession(msk(2026, 1, 1, 7), tp(msk(2026, 1, 1, 13)), periods)
	if len(res) != 2 || !res[0].Start.Equal(msk(2026, 1, 1, 8)) || !endEq(res[0].End, msk(2026, 1, 1, 10)) ||
		!res[1].Start.Equal(msk(2026, 1, 1, 12)) || !endEq(res[1].End, msk(2026, 1, 1, 13)) {
		t.Fatalf("got %+v", res)
	}
	res2 := summary.ClipSession(msk(2026, 1, 1, 9), nil, periods)
	if len(res2) != 2 || !endEq(res2[0].End, msk(2026, 1, 1, 10)) || res2[1].End != nil {
		t.Fatalf("got %+v", res2)
	}
	res3 := summary.ClipSession(msk(2026, 1, 1, 10, 30), tp(msk(2026, 1, 1, 11)), periods)
	if len(res3) != 0 {
		t.Fatalf("got %+v", res3)
	}
}

func TestLampNeed(t *testing.T) {
	if summary.LampNeed([]float64{4.0, 2.0}) != 4.0 {
		t.Fatal("max")
	}
	if summary.LampNeed(nil) != 0.0 {
		t.Fatal("empty")
	}
	if summary.LampNeed([]float64{0.0}) != 0.0 {
		t.Fatal("zero")
	}
}

func TestPlanMorning(t *testing.T) {
	sunrise := msk(2026, 1, 15, 9)
	day := mkdate(2026, 1, 15)
	notB := msk(2026, 1, 15, 6)
	w := summary.PlanMorning(4, &sunrise, notB, day, mskLoc)
	if w == nil || !w.Start.Equal(msk(2026, 1, 15, 7)) || !w.End.Equal(sunrise) {
		t.Fatalf("got %+v", w)
	}
	// not_before bound
	w2 := summary.PlanMorning(10, &sunrise, notB, day, mskLoc)
	if w2 == nil || !w2.Start.Equal(msk(2026, 1, 15, 6)) {
		t.Fatalf("got %+v", w2)
	}
	// summer / zero / no sunrise -> nil
	summer := mkdate(2026, 6, 15)
	if summary.PlanMorning(4, tp(msk(2026, 6, 15, 4, 30)), notB, summer, mskLoc) != nil {
		t.Fatal("summer nil")
	}
	if summary.PlanMorning(0, &sunrise, notB, day, mskLoc) != nil {
		t.Fatal("zero nil")
	}
	if summary.PlanMorning(4, nil, notB, day, mskLoc) != nil {
		t.Fatal("no sunrise nil")
	}
}

func TestPlanEvening(t *testing.T) {
	sunset := msk(2026, 1, 15, 16, 30)
	day := mkdate(2026, 1, 15)
	notA := msk(2026, 1, 15, 23)
	w := summary.PlanEvening(2.5, &sunset, notA, day, mskLoc)
	if w == nil || !w.Start.Equal(sunset) || !w.End.Equal(msk(2026, 1, 15, 19)) {
		t.Fatalf("got %+v", w)
	}
	w2 := summary.PlanEvening(8, &sunset, notA, day, mskLoc)
	if w2 == nil || !w2.End.Equal(msk(2026, 1, 15, 23)) {
		t.Fatalf("got %+v", w2)
	}
	if summary.PlanEvening(0, &sunset, notA, day, mskLoc) != nil {
		t.Fatal("zero nil")
	}
	if summary.PlanEvening(2, tp(msk(2026, 1, 15, 23, 30)), notA, day, mskLoc) != nil {
		t.Fatal("after nil")
	}
	if summary.PlanEvening(2, nil, notA, day, mskLoc) != nil {
		t.Fatal("no sunset nil")
	}
}

func TestPlanDay(t *testing.T) {
	sunrise := msk(2026, 1, 15, 9)
	sunset := msk(2026, 1, 15, 16, 30)
	w := summary.PlanDay(0.5, &sunrise, &sunset)
	if w == nil || !w.Start.Equal(msk(2026, 1, 15, 16)) || !w.End.Equal(sunset) {
		t.Fatalf("got %+v", w)
	}
	w2 := summary.PlanDay(10, &sunrise, &sunset)
	if w2 == nil || !w2.Start.Equal(sunrise) || !w2.End.Equal(sunset) {
		t.Fatalf("got %+v", w2)
	}
	if summary.PlanDay(0, &sunrise, &sunset) != nil {
		t.Fatal("zero nil")
	}
	if summary.PlanDay(1, nil, &sunset) != nil {
		t.Fatal("no sunrise nil")
	}
	if summary.PlanDay(1, &sunrise, nil) != nil {
		t.Fatal("no sunset nil")
	}
}
