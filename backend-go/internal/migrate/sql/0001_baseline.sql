-- Полная схема, идентичная Alembic head (0001–0005). Применяется только на пустой базе;
-- на существующей (Alembic) схеме миграция пропускается.

CREATE TYPE season AS ENUM ('active', 'dormant');
CREATE TYPE feed_method AS ENUM ('root', 'foliar');
CREATE TYPE lamp_mode AS ENUM ('auto', 'schedule', 'manual');
CREATE TYPE lamp_source AS ENUM ('manual', 'schedule', 'auto');

CREATE TABLE users (
    id serial PRIMARY KEY,
    email varchar(254) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    is_admin boolean NOT NULL DEFAULT false,
    blocked_at timestamptz,
    token_version integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE plants (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name varchar(100) NOT NULL,
    species varchar(100) NOT NULL DEFAULT '',
    location varchar(100),
    pot_size_l double precision,
    added_at timestamptz NOT NULL DEFAULT now(),
    notes text,
    water_interval_days integer NOT NULL DEFAULT 4,
    fertilizing_enabled boolean NOT NULL DEFAULT true,
    light_target_hours double precision NOT NULL DEFAULT 12,
    repot_check_interval_months integer NOT NULL DEFAULT 12,
    CONSTRAINT water_interval_positive CHECK (water_interval_days > 0),
    CONSTRAINT light_target_range CHECK (light_target_hours BETWEEN 0 AND 24),
    CONSTRAINT repot_interval_positive CHECK (repot_check_interval_months > 0)
);
CREATE INDEX ix_plants_user_id ON plants(user_id);

CREATE TABLE fertilizer_types (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name varchar(100) NOT NULL,
    npk varchar(50) NOT NULL DEFAULT '',
    root_dose_ml_per_l double precision,
    foliar_dose_ml_per_l double precision,
    interval_days_active_season integer NOT NULL,
    interval_days_dormant_season integer,
    CONSTRAINT uq_fertilizer_user_name UNIQUE (user_id, name)
);
CREATE INDEX ix_fertilizer_types_user_id ON fertilizer_types(user_id);

CREATE TABLE watering_logs (
    id serial PRIMARY KEY,
    plant_id integer NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    watered_at timestamptz NOT NULL DEFAULT now(),
    note text
);
CREATE INDEX ix_watering_logs_plant_id ON watering_logs(plant_id);

CREATE TABLE soil_checks (
    id serial PRIMARY KEY,
    plant_id integer NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    checked_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_soil_checks_plant_id ON soil_checks(plant_id);

CREATE TABLE feeding_logs (
    id serial PRIMARY KEY,
    plant_id integer NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    fertilizer_type_id integer REFERENCES fertilizer_types(id) ON DELETE SET NULL,
    method feed_method NOT NULL,
    fed_at timestamptz NOT NULL DEFAULT now(),
    note text
);
CREATE INDEX ix_feeding_logs_plant_id ON feeding_logs(plant_id);

CREATE TABLE repotting_logs (
    id serial PRIMARY KEY,
    plant_id integer NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    repotted_at timestamptz NOT NULL DEFAULT now(),
    pot_size_before double precision,
    pot_size_after double precision,
    note text
);
CREATE INDEX ix_repotting_logs_plant_id ON repotting_logs(plant_id);

CREATE TABLE lamps (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name varchar(100) NOT NULL,
    mode lamp_mode NOT NULL DEFAULT 'manual',
    device_id varchar(100),
    device_name varchar(200),
    morning_not_before time NOT NULL DEFAULT '06:00:00',
    evening_not_after time NOT NULL DEFAULT '23:00:00',
    last_state boolean,
    last_error text,
    last_error_at timestamptz,
    archived_at timestamptz,
    paused_until timestamptz,
    CONSTRAINT lamp_bounds_order CHECK (evening_not_after > morning_not_before)
);
CREATE INDEX ix_lamps_user_id ON lamps(user_id);

CREATE TABLE plant_lamps (
    id serial PRIMARY KEY,
    plant_id integer NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    lamp_id integer NOT NULL REFERENCES lamps(id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    CONSTRAINT plant_lamp_end_after_start CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE INDEX ix_plant_lamps_plant_id ON plant_lamps(plant_id);
CREATE INDEX ix_plant_lamps_lamp_id ON plant_lamps(lamp_id);
CREATE UNIQUE INDEX uq_plant_lamp_open ON plant_lamps (plant_id) WHERE ended_at IS NULL;

CREATE TABLE lamp_schedules (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lamp_id integer NOT NULL REFERENCES lamps(id) ON DELETE CASCADE,
    start_time time NOT NULL,
    end_time time NOT NULL,
    CONSTRAINT schedule_end_after_start CHECK (end_time > start_time)
);
CREATE INDEX ix_lamp_schedules_user_id ON lamp_schedules(user_id);
CREATE INDEX ix_lamp_schedules_lamp_id ON lamp_schedules(lamp_id);

CREATE TABLE lamp_sessions (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lamp_id integer NOT NULL REFERENCES lamps(id) ON DELETE CASCADE,
    source lamp_source NOT NULL DEFAULT 'manual',
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    planned_hours_per_day double precision NOT NULL DEFAULT 12,
    schedule_id integer REFERENCES lamp_schedules(id) ON DELETE SET NULL,
    after_pause boolean NOT NULL DEFAULT false,
    CONSTRAINT lamp_end_after_start CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE INDEX ix_lamp_sessions_user_id ON lamp_sessions(user_id);
CREATE INDEX ix_lamp_sessions_lamp_id ON lamp_sessions(lamp_id);
CREATE INDEX ix_lamp_sessions_schedule_id ON lamp_sessions(schedule_id);
CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (lamp_id) WHERE ended_at IS NULL;

CREATE TABLE daylight_days (
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day date NOT NULL,
    sunrise timestamptz,
    sunset timestamptz,
    daylight_hours double precision NOT NULL,
    sunshine_hours double precision NOT NULL,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, day)
);

CREATE TABLE user_settings (
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    current_season season NOT NULL DEFAULT 'active',
    notify_days_ahead integer NOT NULL DEFAULT 1,
    location_name varchar(200),
    latitude double precision,
    longitude double precision,
    yandex_token text,
    yandex_token_invalid boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id)
);

-- Стартовые данные (как в Alembic 0001/0002): владелец-заглушка, лимон и лайм, два удобрения.
INSERT INTO users (id, email, password_hash, is_admin) VALUES (1, 'owner@localhost.invalid', '!', true);
SELECT setval('users_id_seq', 1);
INSERT INTO plants (user_id, name, species, water_interval_days, light_target_hours)
VALUES (1, 'Лимон', 'Лимон Мейера', 4, 12), (1, 'Лайм', 'Лайм', 3, 12);
INSERT INTO fertilizer_types (user_id, name, interval_days_active_season, interval_days_dormant_season)
VALUES (1, 'Lomonosoff', 14, 30), (1, 'Bona Forte', 14, 30);
INSERT INTO user_settings (user_id) VALUES (1);
