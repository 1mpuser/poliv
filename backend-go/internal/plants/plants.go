// Сборка сводки, истории и статистики растения из БД. Порт app/services/plants.py.
package plants

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"poliv/internal/lamps"
	"poliv/internal/models"
	"poliv/internal/render"
	"poliv/internal/schema"
	"poliv/internal/summary"
	"poliv/internal/users"
)

type Service struct {
	Pool   *pgxpool.Pool
	Zone   *time.Location
	Lamps  *lamps.Service
}

func New(pool *pgxpool.Pool, zone *time.Location, lamps *lamps.Service) *Service {
	return &Service{Pool: pool, Zone: zone, Lamps: lamps}
}

func (s *Service) GetPlant(ctx context.Context, id int) (*models.Plant, error) {
	row := s.Pool.QueryRow(ctx, `SELECT id, user_id, name, species, location, pot_size_l, added_at, notes,
		water_interval_days, fertilizing_enabled, light_target_hours, repot_check_interval_months
		FROM plants WHERE id=$1`, id)
	return scanPlant(row)
}

func scanPlant(row pgx.Row) (*models.Plant, error) {
	var p models.Plant
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Species, &p.Location, &p.PotSizeL, &p.AddedAt, &p.Notes,
		&p.WaterIntervalDays, &p.FertilizingEnabled, &p.LightTargetHours, &p.RepotCheckIntervalMonths)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) PlantsByUser(ctx context.Context, userID int) ([]models.Plant, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, user_id, name, species, location, pot_size_l, added_at, notes,
		water_interval_days, fertilizing_enabled, light_target_hours, repot_check_interval_months
		FROM plants WHERE user_id=$1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Plant
	for rows.Next() {
		p, err := scanPlant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Service) FertilizersForUser(ctx context.Context, userID int) ([]models.FertilizerType, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, user_id, name, npk, root_dose_ml_per_l, foliar_dose_ml_per_l,
		interval_days_active_season, interval_days_dormant_season
		FROM fertilizer_types WHERE user_id=$1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.FertilizerType
	for rows.Next() {
		var f models.FertilizerType
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &f.NPK, &f.RootDoseMlPerL, &f.FoliarDoseMlPerL,
			&f.IntervalDaysActiveSeason, &f.IntervalDaysDormantSeason); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func toSummaryFerts(fts []models.FertilizerType) []summary.Fertilizer {
	out := make([]summary.Fertilizer, len(fts))
	for i, f := range fts {
		out[i] = summary.Fertilizer{
			ID: f.ID, Name: f.Name, NPK: f.NPK,
			RootDoseMlPerL: f.RootDoseMlPerL, FoliarDoseMlPerL: f.FoliarDoseMlPerL,
			IntervalDaysActiveSeason: f.IntervalDaysActiveSeason,
			IntervalDaysDormantSeason: f.IntervalDaysDormantSeason,
		}
	}
	return out
}

// BuildSummary — сводка одного растения.
func (s *Service) BuildSummary(ctx context.Context, plantID, userID int) (*schema.PlantSummary, error) {
	plant, err := s.GetPlant(ctx, plantID)
	if err != nil {
		return nil, err
	}
	settings, err := users.GetUserSettings(ctx, s.Pool, userID)
	if err != nil {
		return nil, err
	}
	ferts, err := s.FertilizersForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	daylight, err := todayDaylight(ctx, s.Pool, userID, s.Zone)
	if err != nil {
		return nil, err
	}
	return s.buildSummary(ctx, plant, settings, toSummaryFerts(ferts), now, daylight)
}

func todayDaylight(ctx context.Context, pool *pgxpool.Pool, userID int, zone *time.Location) (*models.DaylightDay, error) {
	today := summary.LocalDate(time.Now().In(zone), zone)
	row := pool.QueryRow(ctx, `SELECT user_id, day, sunrise, sunset, daylight_hours, sunshine_hours, fetched_at
		FROM daylight_days WHERE user_id=$1 AND day=$2`, userID, today)
	var d models.DaylightDay
	err := row.Scan(&d.UserID, &d.Day, &d.Sunrise, &d.Sunset, &d.DaylightHours, &d.SunshineHours, &d.FetchedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Service) buildSummary(ctx context.Context, plant *models.Plant, settings *models.UserSettings,
	ferts []summary.Fertilizer, now time.Time, daylight *models.DaylightDay) (*schema.PlantSummary, error) {
	zone := s.Zone
	ahead := settings.NotifyDaysAhead

	var lastWater, lastCheck *time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT max(watered_at) FROM watering_logs WHERE plant_id=$1`, plant.ID).Scan(&lastWater); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT max(checked_at) FROM soil_checks WHERE plant_id=$1`, plant.ID).Scan(&lastCheck); err != nil {
		return nil, err
	}
	water := summary.WaterStateOf(lastWater, lastCheck, plant.WaterIntervalDays, ahead, now, zone)

	var lastFed models.FeedingLog
	lastFed, lastFertName, err := s.lastFeeding(ctx, plant.ID)
	if err != nil {
		return nil, err
	}
	feed := summary.FeedStateOf(plant.FertilizingEnabled, ferts, feedPtr(lastFed.FedAt), lastFed.FertilizerTypeID,
		settings.CurrentSeason, ahead, now, zone)

	midnight := summary.LocalMidnight(summary.LocalDate(now, zone), zone)
	ps, err := s.Lamps.PlantSessions(ctx, s.Pool, plant.ID, &midnight)
	if err != nil {
		return nil, err
	}
	sessions := lamps.AsRules(ps)
	hours := summary.LampHoursToday(sessions, now, zone)

	lampID, err := s.Lamps.CurrentLampID(ctx, plant.ID)
	if err != nil {
		return nil, err
	}
	var lamp *models.Lamp
	if lampID != nil {
		lamp, err = s.Lamps.GetLamp(ctx, *lampID)
		if err != nil {
			return nil, err
		}
	}
	var current *models.LampSession
	if lampID != nil {
		current, err = s.Lamps.CoveringSession(ctx, *lampID, now)
		if err != nil {
			return nil, err
		}
	}

	var lastRepot *time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT max(repotted_at) FROM repotting_logs WHERE plant_id=$1`, plant.ID).Scan(&lastRepot); err != nil {
		return nil, err
	}
	repot := summary.RepotStateOf(lastRepot, plant.AddedAt, plant.RepotCheckIntervalMonths, now, zone)

	plantOut := plantToOut(plant, zone)
	feedOut := feedSummary(plant, lastFed, lastFertName, feed, zone)
	feedOut.Next = fertToSchema(ferts, feed.Next)

	brief, err := s.Lamps.Brief(ctx, lamp, now)
	if err != nil {
		return nil, err
	}
	lightSum := s.buildLight(plant, settings, daylight, sessions, now, brief)
	var openID *int
	if current != nil {
		openID = &current.ID
	}

	var repotLastAt *string
	if lastRepot != nil {
		r := render.Time(*lastRepot, zone)
		repotLastAt = &r
	}

	return &schema.PlantSummary{
		Plant:  *plantOut,
		Season: settings.CurrentSeason,
		Water: schema.WaterSummary{
			LastAt: optTimeStr(lastWater, zone), DaysSince: water.DaysSince,
			LastCheckAt: optTimeStr(lastCheck, zone), DaysSinceCheck: water.DaysSinceCheck,
			IntervalDays: plant.WaterIntervalDays, DueInDays: water.DueInDays, Status: water.Status,
		},
		Feed: feedOut,
		Lamp: schema.LampSummary{
			HoursToday: render.F(hours), PlannedHours: render.F(plant.LightTargetHours),
			Status: summary.LampStatus(hours, plant.LightTargetHours), IsOn: current != nil, OpenSessionID: openID,
		},
		Light: lightSum,
		Repot: schema.RepotSummary{
			LastAt: repotLastAt, IntervalMonths: plant.RepotCheckIntervalMonths,
			NextCheckDate: render.Date(repot.NextCheckDate), DueInDays: repot.DueInDays, Status: repot.Status,
		},
	}, nil
}

func (s *Service) lastFeeding(ctx context.Context, plantID int) (models.FeedingLog, *string, error) {
	row := s.Pool.QueryRow(ctx, `SELECT f.id, f.plant_id, f.fertilizer_type_id, f.method::text, f.fed_at, f.note, ft.name
		FROM feeding_logs f LEFT JOIN fertilizer_types ft ON ft.id=f.fertilizer_type_id
		WHERE f.plant_id=$1 ORDER BY f.fed_at DESC LIMIT 1`, plantID)
	var f models.FeedingLog
	var fertName *string
	err := row.Scan(&f.ID, &f.PlantID, &f.FertilizerTypeID, &f.Method, &f.FedAt, &f.Note, &fertName)
	if err == pgx.ErrNoRows {
		return models.FeedingLog{}, nil, nil
	}
	if err != nil {
		return models.FeedingLog{}, nil, err
	}
	return f, fertName, nil
}

func (s *Service) buildLight(plant *models.Plant, settings *models.UserSettings, daylight *models.DaylightDay,
	sessions []summary.Session, now time.Time, brief *schema.LampBrief) schema.LightSummary {
	zone := s.Zone
	today := summary.LocalDate(now, zone)
	lampHours := summary.LampHoursInDay(sessions, today, now, zone, true)
	var natural *float64
	if daylight != nil {
		n := daylight.SunshineHours
		natural = &n
	}
	state := summary.LightStateOf(plant.LightTargetHours, natural, lampHours)
	var window *summary.LampWindow
	var ends []time.Time
	for _, se := range sessions {
		e := now
		if se.End != nil {
			e = *se.End
		}
		ends = append(ends, e)
	}
	var sunset *time.Time
	if daylight != nil {
		sunset = daylight.Sunset
	}
	window = summary.SuggestLampWindow(state.DeficitHours, sunset, ends, today, zone)
	if window != nil && !window.End.After(now) {
		window = nil
	}
	return schema.LightSummary{
		TargetHours: render.F(plant.LightTargetHours),
		LocationName: settings.LocationName,
		NaturalHours: render.MarshalF(natural),
		DaylightHours: render.MarshalF(daylightHours(daylight)),
		Sunrise: optTime(daylightSunrise(daylight), zone),
		Sunset: optTime(daylightSunset(daylight), zone),
		LampHours: render.F(round2(lampHours)),
		TotalHours: render.F(round2(state.TotalHours)),
		DeficitHours: render.F(round2(state.DeficitHours)),
		Status: state.Status,
		SuggestionStart: optWindowStart(window, zone),
		SuggestionEnd: optWindowEnd(window, zone),
		SuggestionUntilMidnight: window != nil && window.UntilMidnight,
		Lamp: brief,
	}
}

func daylightHours(d *models.DaylightDay) *float64 {
	if d == nil {
		return nil
	}
	v := d.DaylightHours
	return &v
}

func daylightSunrise(d *models.DaylightDay) *time.Time {
	if d == nil {
		return nil
	}
	return d.Sunrise
}

func daylightSunset(d *models.DaylightDay) *time.Time {
	if d == nil {
		return nil
	}
	return d.Sunset
}

func optTime(t *time.Time, zone *time.Location) *string {
	if t == nil {
		return nil
	}
	s := render.Time(*t, zone)
	return &s
}

func optWindowStart(w *summary.LampWindow, zone *time.Location) *string {
	if w == nil {
		return nil
	}
	s := render.Time(w.Start, zone)
	return &s
}

func optWindowEnd(w *summary.LampWindow, zone *time.Location) *string {
	if w == nil {
		return nil
	}
	s := render.Time(w.End, zone)
	return &s
}

func optTimeStr(t *time.Time, zone *time.Location) *string {
	if t == nil {
		return nil
	}
	s := render.Time(*t, zone)
	return &s
}

func plantToOut(p *models.Plant, zone *time.Location) *schema.PlantOut {
	return &schema.PlantOut{
		PlantFields: schema.PlantFields{
			Name: p.Name, Species: p.Species, Location: p.Location,
			PotSizeL: render.MarshalF(p.PotSizeL), Notes: p.Notes,
			WaterIntervalDays: p.WaterIntervalDays, FertilizingEnabled: p.FertilizingEnabled,
			LightTargetHours: render.F(p.LightTargetHours), RepotCheckMonths: p.RepotCheckIntervalMonths,
		},
		ID: p.ID, AddedAt: render.Time(p.AddedAt, zone),
	}
}

func feedSummary(plant *models.Plant, lastFed models.FeedingLog, lastFertName *string, feed summary.FeedState, zone *time.Location) schema.FeedSummary {
	out := schema.FeedSummary{
		Enabled: plant.FertilizingEnabled, DaysSince: feed.DaysSince,
		IntervalDays: feed.IntervalDays, DueInDays: feed.DueInDays, Status: feed.Status,
		LastFertilizerName: lastFertName,
	}
	if !lastFed.FedAt.IsZero() {
		v := render.Time(lastFed.FedAt, zone)
		out.LastAt = &v
	}
	if feed.DueDate != nil {
		v := render.Date(*feed.DueDate)
		out.DueDate = &v
	}
	return out
}

func fertToSchema(ferts []summary.Fertilizer, next *summary.Fertilizer) *schema.Fertilizer {
	if next == nil {
		return nil
	}
	return &schema.Fertilizer{
		ID: next.ID, Name: next.Name, NPK: next.NPK,
		RootDoseMlPerL: render.MarshalF(next.RootDoseMlPerL), FoliarDoseMlPerL: render.MarshalF(next.FoliarDoseMlPerL),
		IntervalDaysActive: next.IntervalDaysActiveSeason, IntervalDaysDormant: next.IntervalDaysDormantSeason,
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// History — лента событий растения. types — фильтр, dateFrom/dateTo — границы (date).
func (s *Service) History(ctx context.Context, plantID int, types map[string]bool, dateFrom, dateTo *time.Time, loc *time.Location) ([]schema.HistoryEvent, error) {
	zone := s.Zone
	now := time.Now().UTC()
	events := []schema.HistoryEvent{}

	// в range по колонке (tz-осведомлённые границы)
	rangeCond := func(col string) (string, []any) {
		cond := ""
		var args []any
		i := 2
		if dateFrom != nil {
			cond += " AND " + col + " >= $" + itoa(i)
			args = append(args, *dateFrom)
			i++
		}
		if dateTo != nil {
			cond += " AND " + col + " < $" + itoa(i)
			args = append(args, *dateTo)
			i++
		}
		return cond, args
	}

	if types["water"] {
		cond, args := rangeCond("watered_at")
		rows, err := s.Pool.Query(ctx, `SELECT id, plant_id, watered_at, note FROM watering_logs WHERE plant_id=$1`+cond,
			append([]any{plantID}, args...)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var w models.WateringLog
			if err := rows.Scan(&w.ID, &w.PlantID, &w.WateredAt, &w.Note); err != nil {
				rows.Close()
				return nil, err
			}
			events = append(events, schema.HistoryEvent{Type: "water", ID: w.ID, At: render.Time(w.WateredAt, zone), Note: w.Note})
		}
		rows.Close()
	}
	if types["check"] {
		cond, args := rangeCond("checked_at")
		rows, err := s.Pool.Query(ctx, `SELECT id, plant_id, checked_at FROM soil_checks WHERE plant_id=$1`+cond,
			append([]any{plantID}, args...)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, pid int
			var at time.Time
			if err := rows.Scan(&id, &pid, &at); err != nil {
				rows.Close()
				return nil, err
			}
			events = append(events, schema.HistoryEvent{Type: "check", ID: id, At: render.Time(at, zone)})
		}
		rows.Close()
	}
	if types["feed"] {
		cond, args := rangeCond("fed_at")
		rows, err := s.Pool.Query(ctx, `SELECT f.id, f.plant_id, f.fed_at, f.note, f.method::text, ft.name
			FROM feeding_logs f LEFT JOIN fertilizer_types ft ON ft.id=f.fertilizer_type_id WHERE f.plant_id=$1`+cond,
			append([]any{plantID}, args...)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, pid int
			var at time.Time
			var note, method *string
			var fertName *string
			if err := rows.Scan(&id, &pid, &at, &note, &method, &fertName); err != nil {
				rows.Close()
				return nil, err
			}
			events = append(events, schema.HistoryEvent{Type: "feed", ID: id, At: render.Time(at, zone), Note: note, FertilizerName: fertName, Method: method})
		}
		rows.Close()
	}
	if types["lamp"] {
		ps, err := s.Lamps.PlantSessions(ctx, s.Pool, plantID, dateFrom)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if dateFrom != nil && p.StartedAt.Before(*dateFrom) {
				continue
			}
			if dateTo != nil && !p.StartedAt.Before(*dateTo) {
				continue
			}
			end := now
			if p.EndedAt != nil {
				end = *p.EndedAt
			}
			hours := end.Sub(p.StartedAt).Hours()
			ev := schema.HistoryEvent{Type: "lamp", ID: p.ID, At: render.Time(p.StartedAt, zone),
				EndedAt: optTime(p.EndedAt, zone), LampName: &p.LampName}
			hv := render.F(round2(hours))
			ev.Hours = &hv
			events = append(events, ev)
		}
	}
	if types["repot"] {
		cond, args := rangeCond("repotted_at")
		rows, err := s.Pool.Query(ctx, `SELECT id, plant_id, repotted_at, note, pot_size_before, pot_size_after
			FROM repotting_logs WHERE plant_id=$1`+cond, append([]any{plantID}, args...)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, pid int
			var at time.Time
			var note *string
			var before, after *float64
			if err := rows.Scan(&id, &pid, &at, &note, &before, &after); err != nil {
				rows.Close()
				return nil, err
			}
			events = append(events, schema.HistoryEvent{Type: "repot", ID: id, At: render.Time(at, zone),
				Note: note, PotSizeBefore: render.MarshalF(before), PotSizeAfter: render.MarshalF(after)})
		}
		rows.Close()
	}

	sortEventsByAtDesc(events)
	return events, nil
}


func sortEventsByAtDesc(events []schema.HistoryEvent) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].At > events[j-1].At; j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}

// Weekly — недельная статистика по растение.
func (s *Service) Weekly(ctx context.Context, userID, plantID, weeks int) ([]schema.WeekStat, error) {
	zone := s.Zone
	now := time.Now().UTC()
	today := summary.LocalDate(now, zone)
	wd := int(today.Weekday())
	offset := (wd + 6) % 7
	since := summary.LocalMidnight(today.AddDate(0, 0, -offset-(weeks-1)*7), zone)

	var waterings, feedings []time.Time
	rows, err := s.Pool.Query(ctx, `SELECT watered_at FROM watering_logs WHERE plant_id=$1 AND watered_at >= $2`, plantID, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		waterings = append(waterings, t)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `SELECT fed_at FROM feeding_logs WHERE plant_id=$1 AND fed_at >= $2`, plantID, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		feedings = append(feedings, t)
	}
	rows.Close()

	sunshine, err := lightSunshineByDay(ctx, s.Pool, userID, since)
	if err != nil {
		return nil, err
	}
	filtered := map[summary.Date]float64{}
	for d, h := range sunshine {
		if d.Before(today) {
			filtered[d] = h
		}
	}
	ps, err := s.Lamps.PlantSessions(ctx, s.Pool, plantID, &since)
	if err != nil {
		return nil, err
	}
	stats := summary.WeeklyStats(waterings, feedings, lamps.AsRules(ps), weeks, now, zone, filtered)
	out := []schema.WeekStat{}
	for _, w := range stats {
		out = append(out, schema.WeekStat{
			WeekStart: render.Date(w.WeekStart), Waterings: w.Waterings, Feedings: w.Feedings,
			LampHours: render.F(round1(w.LampHours)), SunshineHours: render.F(round1(w.SunshineHours)),
			IsCurrent: w.IsCurrent,
		})
	}
	return out, nil
}

func lightSunshineByDay(ctx context.Context, pool *pgxpool.Pool, userID int, since time.Time) (map[summary.Date]float64, error) {
	rows, err := pool.Query(ctx, `SELECT day, sunshine_hours FROM daylight_days WHERE user_id=$1 AND day >= $2`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[summary.Date]float64{}
	for rows.Next() {
		var d summary.Date
		var h float64
		if err := rows.Scan(&d, &h); err != nil {
			return nil, err
		}
		out[d] = h
	}
	return out, rows.Err()
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func feedPtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// ScanPlant — публичная обёртка чтения строки plants.
func ScanPlant(row pgx.Row) (*models.Plant, error) {
	return scanPlant(row)
}

// BuildOne — сводка одного растения с готовыми настройками/удобрениями/светом.
func (s *Service) BuildOne(ctx context.Context, plant *models.Plant, settings *models.UserSettings,
	ferts []summary.Fertilizer, now time.Time, daylight *models.DaylightDay) (*schema.PlantSummary, error) {
	return s.buildSummary(ctx, plant, settings, ferts, now, daylight)
}

// CurrentDaylight — свет сегодня в городе учётки.
func (s *Service) CurrentDaylight(ctx context.Context, userID int) (*models.DaylightDay, error) {
	return todayDaylight(ctx, s.Pool, userID, s.Zone)
}

func ToSummaryFerts(fts []models.FertilizerType) []summary.Fertilizer {
	return toSummaryFerts(fts)
}
