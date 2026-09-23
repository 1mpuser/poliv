export type Status = 'ok' | 'soon' | 'late' | 'off';
export type Season = 'active' | 'dormant';
export type FeedMethod = 'root' | 'foliar';
export type EventType = 'water' | 'feed' | 'lamp' | 'repot';

export interface PlantFields {
  name: string;
  species: string;
  location: string | null;
  pot_size_l: number | null;
  notes: string | null;
  water_interval_days: number;
  fertilizing_enabled: boolean;
  /** Норма всего света в день: солнце + лампа */
  light_target_hours: number;
  repot_check_interval_months: number;
}

export interface Plant extends PlantFields {
  id: number;
  added_at: string;
}

export interface FertilizerFields {
  name: string;
  npk: string;
  root_dose_ml_per_l: number | null;
  foliar_dose_ml_per_l: number | null;
  interval_days_active_season: number;
  interval_days_dormant_season: number | null;
}

export interface Fertilizer extends FertilizerFields {
  id: number;
}

export interface PlantSummary {
  plant: Plant;
  season: Season;
  water: {
    last_at: string | null;
    days_since: number | null;
    interval_days: number;
    due_in_days: number;
    status: Status;
  };
  feed: {
    enabled: boolean;
    last_at: string | null;
    last_fertilizer_name: string | null;
    days_since: number | null;
    next: Fertilizer | null;
    interval_days: number | null;
    due_date: string | null;
    due_in_days: number | null;
    status: Status;
  };
  lamp: {
    hours_today: number;
    planned_hours: number;
    status: Status;
    is_on: boolean;
    open_session_id: number | null;
    shared_is_on: boolean;
  };
  light: LightSummary;
  repot: {
    last_at: string | null;
    interval_months: number;
    next_check_date: string;
    due_in_days: number;
    status: Status;
  };
}

export interface AppSettings {
  current_season: Season;
  notify_days_ahead: number;
  location_name: string | null;
  latitude: number | null;
  longitude: number | null;
}

/** Интервал расписания, местное время 'HH:MM[:SS]' */
export interface ScheduleInterval {
  start_time: string;
  end_time: string;
}

export interface LampSchedule extends ScheduleInterval {
  id: number;
  plant_id: number | null;
}

export interface LightSummary {
  target_hours: number;
  location_name: string | null;
  natural_hours: number | null;
  daylight_hours: number | null;
  sunrise: string | null;
  sunset: string | null;
  lamp_hours: number;
  total_hours: number;
  deficit_hours: number;
  status: Status;
  suggestion_start: string | null;
  suggestion_end: string | null;
  suggestion_until_midnight: boolean;
  schedule: ScheduleInterval[];
  shared_schedule: ScheduleInterval[];
}

export interface Place {
  name: string;
  region: string | null;
  country: string | null;
  latitude: number;
  longitude: number;
}

export interface Daylight {
  day: string;
  sunrise: string | null;
  sunset: string | null;
  daylight_hours: number;
  sunshine_hours: number;
}

export interface LampSession {
  id: number;
  plant_id: number | null;
  started_at: string;
  ended_at: string | null;
  planned_hours_per_day: number;
}

export interface HistoryEvent {
  type: EventType;
  id: number;
  at: string;
  note: string | null;
  fertilizer_name: string | null;
  method: FeedMethod | null;
  ended_at: string | null;
  hours: number | null;
  shared: boolean | null;
  pot_size_before: number | null;
  pot_size_after: number | null;
}

export interface WeekStat {
  week_start: string;
  waterings: number;
  feedings: number;
  lamp_hours: number;
  sunshine_hours: number;
  is_current: boolean;
}

export interface Me {
  id: number;
  email: string;
  is_admin: boolean;
}

export interface AdminUser {
  id: number;
  email: string;
  is_admin: boolean;
  blocked_at: string | null;
  created_at: string;
}
