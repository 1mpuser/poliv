import { useState } from 'react';
import { api } from '../api';
import { fmtHours, fmtTime } from '../format';
import type { AppSettings, LampSchedule, Place } from '../types';
import { useAsync } from '../useAsync';
import { LampScheduleEditor } from './LampScheduleEditor';
import { useToast } from './toast';

/** Город для светового дня и расписание общей лампы. Сохраняется сразу. */
export function LightSettings({ initial, schedules }: { initial: AppSettings; schedules: LampSchedule[] }) {
  const toast = useToast();
  const [settings, setSettings] = useState(initial);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<Place[] | null>(null);
  const [busy, setBusy] = useState(false);
  const { data: today } = useAsync(() => api.lightToday(), [settings.latitude, settings.longitude]);

  async function search() {
    if (query.trim().length < 2) return;
    setBusy(true);
    try {
      setResults(await api.geocode(query.trim()));
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Поиск не удался');
    } finally {
      setBusy(false);
    }
  }

  async function choose(p: Place) {
    const name = [p.name, p.region].filter(Boolean).join(', ');
    try {
      setSettings(await api.updateSettings({ location_name: name, latitude: p.latitude, longitude: p.longitude }));
      setResults(null);
      setQuery('');
      toast(`Город: ${name}`);
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сохранить город');
    }
  }

  return (
    <>
      <div className="form-group">
        <div className="field field--stack">
          <span className="field__label">Город</span>
          <span className="field__hint">
            {settings.location_name
              ? today
                ? `${settings.location_name}: восход ${fmtTime(today.sunrise!)}, закат ${fmtTime(today.sunset!)}, день ${fmtHours(today.daylight_hours)} ч, солнца ${fmtHours(today.sunshine_hours)} ч`
                : `${settings.location_name}: данные о свете ещё не получены`
              : 'Не задан — солнце не учитывается в норме света'}
          </span>
          <div className="input-row" onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); search(); } }}>
            <input className="input" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Мытищи" aria-label="Поиск города" />
            <button className="btn btn--sm" type="button" disabled={busy} onClick={search}>Найти</button>
          </div>
          {results && (
            <div className="city-results">
              {results.length === 0 && <span className="field__hint">Ничего не нашлось</span>}
              {results.map((p) => (
                <button className="btn btn--sm" type="button" key={`${p.latitude},${p.longitude}`} onClick={() => choose(p)}>
                  {p.name}{p.region ? `, ${p.region}` : ''}{p.country ? ` · ${p.country}` : ''}
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="field field--stack">
          <span className="field__label">Общая лампа — расписание розетки</span>
          <span className="field__hint">Светит на все растения. Своя лампа растения — в его настройках.</span>
          <LampScheduleEditor plantId={null} initial={schedules.filter((s) => s.plant_id === null)} />
        </div>
      </div>
      <p className="field__hint" style={{ marginTop: 8 }}>
        Свет считается по часам прямого солнца с учётом облачности (Open-Meteo) плюс часы лампы.
        На подоконнике света меньше, чем на улице, — норму можно ставить с запасом.
      </p>
    </>
  );
}
