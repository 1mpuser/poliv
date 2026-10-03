-- Корневая подкормка — всегда полив раствором. Тот полив, что создан автоматически,
-- привязан к подкормке (watering_id). Удаление полива оставляет подкормку (ON DELETE SET NULL),
-- удаление корневой подкормки убирает и автополив — это делает код API.
ALTER TABLE feeding_logs ADD COLUMN watering_id integer REFERENCES watering_logs(id) ON DELETE SET NULL;
CREATE INDEX ix_feeding_logs_watering_id ON feeding_logs(watering_id);
