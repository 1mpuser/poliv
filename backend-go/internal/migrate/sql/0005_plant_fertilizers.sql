-- Набор удобрений привязан к растению: каждый вид подкармливает только свои культуры.
-- Существующим растениям бэкфилл привязывает все удобрения их учётки — их поведение
-- (чередование всех удобрений типа) не меняется. Новым — пока пусто, пока не выберут.
CREATE TABLE plant_fertilizers (
    plant_id integer NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    fertilizer_type_id integer NOT NULL REFERENCES fertilizer_types(id) ON DELETE CASCADE,
    PRIMARY KEY (plant_id, fertilizer_type_id)
);
CREATE INDEX ix_plant_fertilizers_plant_id ON plant_fertilizers(plant_id);
CREATE INDEX ix_plant_fertilizers_fertilizer_type_id ON plant_fertilizers(fertilizer_type_id);

INSERT INTO plant_fertilizers (plant_id, fertilizer_type_id)
SELECT p.id, f.id FROM plants p JOIN fertilizer_types f ON f.user_id = p.user_id;

-- Свой интервал подкормки у растения: заменяет интервал удобрения и в активный, и в спящий сезон.
-- NULL (по умолчанию) — как сейчас, интервалы берутся из типа удобрения.
ALTER TABLE plants ADD COLUMN fertilizing_interval_days integer;
ALTER TABLE plants ADD CONSTRAINT fertilizing_interval_positive
    CHECK (fertilizing_interval_days IS NULL OR fertilizing_interval_days > 0);
