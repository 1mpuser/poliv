-- Go-порт писал в daylight_days длительности Open-Meteo в секундах вместо часов.
-- В сутках не больше 24 часов, так что всё, что больше, — секунды.
UPDATE daylight_days SET daylight_hours = round((daylight_hours / 3600)::numeric, 2) WHERE daylight_hours > 24;
UPDATE daylight_days SET sunshine_hours = round((sunshine_hours / 3600)::numeric, 2) WHERE sunshine_hours > 24;
