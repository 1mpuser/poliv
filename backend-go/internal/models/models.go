package models

import "time"

// Структуры строк БД. Все времена timestamptz; pgx отдаёт их в UTC — конвертируем в зону на месте.
// NULL-поля — указатели.

type User struct {
	ID           int
	Email        string
	PasswordHash string
	IsAdmin      bool
	BlockedAt    *time.Time
	TokenVersion int
	CreatedAt    time.Time
}

type Plant struct {
	ID                         int
	UserID                     int
	Name                       string
	Species                    string
	Location                   *string
	PotSizeL                   *float64
	AddedAt                    time.Time
	Notes                      *string
	WaterIntervalDays          int
	FertilizingEnabled         bool
	LightTargetHours           float64
	RepotCheckIntervalMonths   int
}

type FertilizerType struct {
	ID                         int
	UserID                     int
	Name                       string
	NPK                        string
	RootDoseMlPerL             *float64
	FoliarDoseMlPerL           *float64
	IntervalDaysActiveSeason   int
	IntervalDaysDormantSeason  *int
}

type WateringLog struct {
	ID        int
	PlantID   int
	WateredAt time.Time
	Note      *string
}

type SoilCheck struct {
	ID        int
	PlantID   int
	CheckedAt time.Time
}

type FeedingLog struct {
	ID               int
	PlantID          int
	FertilizerTypeID *int
	Method           string
	FedAt            time.Time
	Note             *string
	// joined fertilizer_type (lazy="joined")
	FertilizerName *string
	FertilizerNPK  *string
	RootDose       *float64
	FoliarDose     *float64
	IntervalActive *int
	IntervalDorm   *int
}

type Lamp struct {
	ID                 int
	UserID             int
	Name               string
	Mode               string
	DeviceID           *string
	DeviceName         *string
	MorningNotBefore   time.Time // TIME колонки
	EveningNotAfter    time.Time
	LastState          *bool
	LastError          *string
	LastErrorAt        *time.Time
	ArchivedAt         *time.Time
	PausedUntil        *time.Time
}

type PlantLamp struct {
	ID        int
	PlantID   int
	LampID    int
	StartedAt time.Time
	EndedAt   *time.Time
}

type LampSession struct {
	ID                 int
	UserID             int
	LampID             int
	Source             string
	StartedAt          time.Time
	EndedAt            *time.Time
	PlannedHours       float64
	ScheduleID         *int
	AfterPause         bool
	// join Lamp.name (для plant_sessions)
	LampName *string
}

type LampSchedule struct {
	ID        int
	UserID    int
	LampID    int
	StartTime time.Time
	EndTime   time.Time
}

type DaylightDay struct {
	UserID        int
	Day           time.Time // DATE
	Sunrise       *time.Time
	Sunset        *time.Time
	DaylightHours float64
	SunshineHours float64
	FetchedAt     time.Time
}

type RepottingLog struct {
	ID            int
	PlantID       int
	RepottedAt    time.Time
	PotSizeBefore *float64
	PotSizeAfter  *float64
	Note          *string
}

type UserSettings struct {
	UserID            int
	CurrentSeason     string
	NotifyDaysAhead   int
	LocationName      *string
	Latitude          *float64
	Longitude         *float64
	YandexToken       *string
	YandexTokenInvalid bool
}

func (s *UserSettings) YandexStatus() string {
	if s.YandexToken == nil {
		return "none"
	}
	if s.YandexTokenInvalid {
		return "invalid"
	}
	return "ok"
}
