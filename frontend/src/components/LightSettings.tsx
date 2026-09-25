import { useState } from 'react';
import { api } from '../api';
import { fmtHours, fmtTime } from '../format';
import type { AppSettings, Place, YandexStatus } from '../types';
import { useAsync } from '../useAsync';
import { useToast } from './toast';

const TOKEN_HINT: Record<YandexStatus, string> = {
  none: 'Не подключено — лампы только считают часы, розетками не управляют',
  ok: 'Подключено — Поливалка включает и выключает розетки ламп',
  invalid: 'Яндекс больше не принимает токен — вставьте новый',
};

/** Токен Умного дома: проверяется у Яндекса при сохранении, обратно не показывается */
function YandexToken({ status, onChange }: { status: YandexStatus; onChange: (s: AppSettings) => void }) {
  const toast = useToast();
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);

  async function save(value: string | null) {
    setBusy(true);
    try {
      onChange(await api.setYandexToken(value));
      setToken('');
      toast(value ? 'Умный дом Яндекса подключён' : 'Токен Яндекса удалён');
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сохранить токен');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="field field--stack">
      <span className="field__label">Умный дом Яндекса</span>
      <span className="field__hint">
        {TOKEN_HINT[status]}. Токен — на{' '}
        <a href="https://oauth.yandex.ru/client/new" target="_blank" rel="noreferrer">oauth.yandex.ru</a>{' '}
        (права «Умный дом»: просмотр и управление), пошагово — в инструкции.
      </span>
      <div className="input-row" onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); if (token.trim().length >= 10) save(token.trim()); } }}>
        <input
          className="input"
          type="password"
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={status === 'none' ? 'y0_…' : 'Новый токен'}
          aria-label="Токен Яндекса"
        />
        <button className="btn btn--sm" type="button" disabled={busy || token.trim().length < 10} onClick={() => save(token.trim())}>
          Проверить
        </button>
      </div>
      {status !== 'none' && (
        <div className="btn-row" style={{ marginTop: 8 }}>
          <button className="btn btn--sm btn--danger" type="button" disabled={busy} onClick={() => save(null)}>Отключить</button>
        </div>
      )}
    </div>
  );
}

/** Город для светового дня и токен Умного дома Яндекса. Сохраняется сразу. */
export function LightSettings({ initial }: { initial: AppSettings }) {
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
        <YandexToken status={settings.yandex_status} onChange={setSettings} />
      </div>
      <p className="field__hint" style={{ marginTop: 8 }}>
        Свет считается по часам прямого солнца с учётом облачности (Open-Meteo) плюс часы лампы.
        На подоконнике света меньше, чем на улице, — норму можно ставить с запасом.
      </p>
    </>
  );
}
