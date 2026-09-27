// Пакет schema — DTO для JSON-ответов. Имена полей и форматы значений повторяют Pydantic (schemas.py).
package schema

import (
	"time"

	"poliv/internal/render"
)

type PlantFields struct {
	Name                  string        `json:"name"`
	Species               string        `json:"species"`
	Location              *string       `json:"location"`
	PotSizeL              *render.F     `json:"pot_size_l"`
	Notes                 *string       `json:"notes"`
	WaterIntervalDays     int           `json:"water_interval_days"`
	FertilizingEnabled    bool          `json:"fertilizing_enabled"`
	LightTargetHours      render.F      `json:"light_target_hours"`
	RepotCheckMonths      int           `json:"repot_check_interval_months"`
}

type PlantOut struct {
	PlantFields
	ID      int    `json:"id"`
	AddedAt string `json:"added_at"`
}

type Fertilizer struct {
	ID                     int        `json:"id"`
	Name                   string     `json:"name"`
	NPK                    string     `json:"npk"`
	RootDoseMlPerL         *render.F  `json:"root_dose_ml_per_l"`
	FoliarDoseMlPerL       *render.F  `json:"foliar_dose_ml_per_l"`
	IntervalDaysActive     int        `json:"interval_days_active_season"`
	IntervalDaysDormant    *int       `json:"interval_days_dormant_season"`
}

type WateringOut struct {
	ID        int    `json:"id"`
	PlantID   int    `json:"plant_id"`
	WateredAt string `json:"watered_at"`
	Note      *string `json:"note"`
}

type SoilCheckOut struct {
	ID        int    `json:"id"`
	PlantID   int    `json:"plant_id"`
	CheckedAt string `json:"checked_at"`
}

type FeedingOut struct {
	ID               int     `json:"id"`
	PlantID          int     `json:"plant_id"`
	FertilizerTypeID *int    `json:"fertilizer_type_id"`
	Method           string  `json:"method"`
	FedAt            string  `json:"fed_at"`
	Note             *string `json:"note"`
}

type RepottingOut struct {
	ID            int       `json:"id"`
	PlantID       int       `json:"plant_id"`
	RepottedAt    string    `json:"repotted_at"`
	PotSizeBefore *render.F `json:"pot_size_before"`
	PotSizeAfter  *render.F `json:"pot_size_after"`
	Note          *string   `json:"note"`
}

type LampSessionOut struct {
	ID                 int      `json:"id"`
	LampID             int      `json:"lamp_id"`
	Source             string   `json:"source"`
	StartedAt          string   `json:"started_at"`
	EndedAt            *string  `json:"ended_at"`
	PlannedHours       render.F `json:"planned_hours_per_day"`
}

type ScheduleInterval struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type PlannedInterval struct {
	Start string  `json:"start"`
	End   *string `json:"end"`
}

type LampFields struct {
	Name              string  `json:"name"`
	Mode              string  `json:"mode"`
	DeviceID          *string `json:"device_id"`
	DeviceName        *string `json:"device_name"`
	MorningNotBefore  string  `json:"morning_not_before"`
	EveningNotAfter   string  `json:"evening_not_after"`
}

type LampOut struct {
	LampFields
	ID           int                `json:"id"`
	LastState    *bool              `json:"last_state"`
	LastError    *string            `json:"last_error"`
	LastErrorAt  *string            `json:"last_error_at"`
	PausedUntil  *string            `json:"paused_until"`
	PlantIDs     []int              `json:"plant_ids"`
	IsOn         bool               `json:"is_on"`
	Schedule     []ScheduleInterval `json:"schedule"`
	Planned      []PlannedInterval  `json:"planned"`
}

type LampBrief struct {
	ID          int                `json:"id"`
	Name        string             `json:"name"`
	Mode        string             `json:"mode"`
	IsOn        bool               `json:"is_on"`
	HasDevice   bool               `json:"has_device"`
	PausedUntil *string            `json:"paused_until"`
	Planned     []PlannedInterval  `json:"planned"`
	LastError   *string            `json:"last_error"`
}

type YandexDevice struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Room *string `json:"room"`
	Type string  `json:"type"`
}

type SettingsOut struct {
	CurrentSeason    string      `json:"current_season"`
	NotifyDaysAhead  int         `json:"notify_days_ahead"`
	LocationName     *string     `json:"location_name"`
	Latitude         *render.F   `json:"latitude"`
	Longitude        *render.F   `json:"longitude"`
	YandexStatus     string      `json:"yandex_status"`
}

type Place struct {
	Name      string     `json:"name"`
	Region    *string    `json:"region"`
	Country   *string    `json:"country"`
	Latitude  render.F   `json:"latitude"`
	Longitude render.F   `json:"longitude"`
}

type DaylightOut struct {
	Day            string   `json:"day"`
	Sunrise        *string  `json:"sunrise"`
	Sunset         *string  `json:"sunset"`
	DaylightHours  render.F `json:"daylight_hours"`
	SunshineHours  render.F `json:"sunshine_hours"`
}

type WaterSummary struct {
	LastAt         *string   `json:"last_at"`
	DaysSince      *int      `json:"days_since"`
	LastCheckAt    *string   `json:"last_check_at"`
	DaysSinceCheck *int      `json:"days_since_check"`
	IntervalDays   int       `json:"interval_days"`
	DueInDays      int       `json:"due_in_days"`
	Status         string    `json:"status"`
}

type FeedSummary struct {
	Enabled             bool        `json:"enabled"`
	LastAt              *string     `json:"last_at"`
	LastFertilizerName  *string     `json:"last_fertilizer_name"`
	DaysSince           *int        `json:"days_since"`
	Next                *Fertilizer `json:"next"`
	IntervalDays        *int        `json:"interval_days"`
	DueDate             *string     `json:"due_date"`
	DueInDays           *int        `json:"due_in_days"`
	Status              string      `json:"status"`
}

type LampSummary struct {
	HoursToday     render.F `json:"hours_today"`
	PlannedHours   render.F `json:"planned_hours"`
	Status         string   `json:"status"`
	IsOn           bool     `json:"is_on"`
	OpenSessionID  *int     `json:"open_session_id"`
}

type LightSummary struct {
	TargetHours           render.F       `json:"target_hours"`
	LocationName          *string        `json:"location_name"`
	NaturalHours          *render.F      `json:"natural_hours"`
	DaylightHours         *render.F      `json:"daylight_hours"`
	Sunrise               *string        `json:"sunrise"`
	Sunset                *string        `json:"sunset"`
	LampHours             render.F       `json:"lamp_hours"`
	TotalHours            render.F       `json:"total_hours"`
	DeficitHours          render.F       `json:"deficit_hours"`
	Status                string         `json:"status"`
	SuggestionStart       *string        `json:"suggestion_start"`
	SuggestionEnd         *string        `json:"suggestion_end"`
	SuggestionUntilMidnight bool         `json:"suggestion_until_midnight"`
	Lamp                  *LampBrief     `json:"lamp"`
}

type RepotSummary struct {
	LastAt        *string `json:"last_at"`
	IntervalMonths int     `json:"interval_months"`
	NextCheckDate string  `json:"next_check_date"`
	DueInDays     int     `json:"due_in_days"`
	Status        string  `json:"status"`
}

type PlantSummary struct {
	Plant  PlantOut      `json:"plant"`
	Season string        `json:"season"`
	Water  WaterSummary  `json:"water"`
	Feed   FeedSummary   `json:"feed"`
	Lamp   LampSummary   `json:"lamp"`
	Light  LightSummary  `json:"light"`
	Repot  RepotSummary  `json:"repot"`
}

type HistoryEvent struct {
	Type             string    `json:"type"`
	ID               int       `json:"id"`
	At               string    `json:"at"`
	Note             *string   `json:"note"`
	FertilizerName   *string   `json:"fertilizer_name"`
	Method           *string   `json:"method"`
	EndedAt          *string   `json:"ended_at"`
	Hours            *render.F `json:"hours"`
	LampName         *string   `json:"lamp_name"`
	PotSizeBefore    *render.F `json:"pot_size_before"`
	PotSizeAfter     *render.F `json:"pot_size_after"`
}

type WeekStat struct {
	WeekStart      string    `json:"week_start"`
	Waterings      int       `json:"waterings"`
	Feedings       int       `json:"feedings"`
	LampHours      render.F  `json:"lamp_hours"`
	SunshineHours  render.F  `json:"sunshine_hours"`
	IsCurrent      bool      `json:"is_current"`
}

type LampToggleOut struct {
	IsOn            bool            `json:"is_on"`
	Session         LampSessionOut  `json:"session"`
	PreviousEndedAt *string         `json:"previous_ended_at"`
	PlugError       *string         `json:"plug_error"`
}

type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type AdminUser struct {
	ID        int     `json:"id"`
	Email     string  `json:"email"`
	IsAdmin   bool    `json:"is_admin"`
	BlockedAt *string `json:"blocked_at"`
	CreatedAt string  `json:"created_at"`
}

type Me struct {
	ID      int    `json:"id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

// ---------- вспомогательные конструкторы ----------

func FT(t time.Time, loc *time.Location) string { return render.Time(t, loc) }
func Str(s *string) *string                    { return s }
func NullStr() *string                         { return nil }
