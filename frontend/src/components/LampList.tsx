import { useState } from 'react';
import { api } from '../api';
import type { Lamp, LampFields, LampMode, Plant } from '../types';
import { useAsync } from '../useAsync';
import { Segmented } from './controls';
import { LampScheduleEditor } from './LampScheduleEditor';
import { useToast } from './toast';

const MODES: { value: LampMode; label: string }[] = [
  { value: 'auto', label: 'Авто' },
  { value: 'schedule', label: 'По расписанию' },
  { value: 'manual', label: 'Вручную' },
];

const MODE_HINT: Record<LampMode, string> = {
  auto: 'Досвечивает до нормы самого требовательного растения: половину утром до рассвета, остаток вечером после заката. Нужны город и розетка.',
  schedule: 'Горит по интервалам — как программируемая розетка.',
  manual: 'Только кнопкой «Лампа».',
};

const hhmm = (t: string) => t.slice(0, 5);

const EMPTY: LampFields = {
  name: '',
  mode: 'manual',
  device_id: null,
  device_name: null,
  morning_not_before: '06:00',
  evening_not_after: '23:00',
};

function meta(l: Lamp, plants: Plant[]): string {
  const names = plants.filter((p) => l.plant_ids.includes(p.id)).map((p) => p.name);
  return [
    MODES.find((m) => m.value === l.mode)?.label,
    l.device_name ? `розетка «${l.device_name}»` : 'без розетки',
    names.length ? names.join(', ') : 'растений нет',
    l.is_on && 'горит',
  ].filter(Boolean).join(' · ');
}

/** Лампы учётки: растения под лампой, режим и розетка. Сохраняется сразу, отдельно от формы настроек. */
export function LampList({ lamps, plants, onChanged }: { lamps: Lamp[]; plants: Plant[]; onChanged: () => void }) {
  const toast = useToast();
  const [editing, setEditing] = useState<number | 'new' | null>(null);
  const done = () => { setEditing(null); onChanged(); };

  async function remove(l: Lamp) {
    if (!confirm(`Удалить «${l.name}»? Растения останутся без лампы, часы в истории сохранятся.`)) return;
    try {
      await api.archiveLamp(l.id);
      done();
    } catch {
      toast('Не удалось удалить');
    }
  }

  return (
    <div className="form-group">
      {lamps.length === 0 && editing !== 'new' && <p className="empty">Ламп пока нет</p>}
      {lamps.map((l) =>
        editing === l.id ? (
          <LampForm key={l.id} lamp={l} lamps={lamps} plants={plants} onCancel={() => setEditing(null)} onSaved={done} />
        ) : (
          <div className="fert-item" key={l.id}>
            <div className="fert-item__text">
              <div className="field__label">{l.name}</div>
              <div className="fert-item__meta">{meta(l, plants)}</div>
              {l.last_error && <div className="form-error">Розетка: {l.last_error}</div>}
            </div>
            <button className="btn btn--sm" type="button" onClick={() => setEditing(l.id)}>Изменить</button>
            <button className="btn btn--sm btn--danger" type="button" aria-label={`Удалить ${l.name}`} onClick={() => remove(l)}>✕</button>
          </div>
        ),
      )}
      {editing === 'new' ? (
        <LampForm lamps={lamps} plants={plants} onCancel={() => setEditing(null)} onSaved={done} />
      ) : (
        <div className="fert-item">
          <button className="btn btn--sm btn--ghost" type="button" onClick={() => setEditing('new')}>+ Добавить лампу</button>
        </div>
      )}
    </div>
  );
}

function LampForm({
  lamp,
  lamps,
  plants,
  onSaved,
  onCancel,
}: {
  lamp?: Lamp;
  lamps: Lamp[];
  plants: Plant[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [f, setF] = useState<LampFields>(
    lamp
      ? {
          name: lamp.name,
          mode: lamp.mode,
          device_id: lamp.device_id,
          device_name: lamp.device_name,
          morning_not_before: hhmm(lamp.morning_not_before),
          evening_not_after: hhmm(lamp.evening_not_after),
        }
      : EMPTY,
  );
  const [ids, setIds] = useState<number[]>(lamp?.plant_ids ?? []);
  const [error, setError] = useState<string | null>(null);
  // Без токена Яндекс ответит 400 с подсказкой — её и показываем
  const { data: devices, error: devicesError } = useAsync(() => api.yandexDevices(), []);

  const otherLamp = (plantId: number) => lamps.find((l) => l.id !== lamp?.id && l.plant_ids.includes(plantId));
  const toggleId = (plantId: number) => setIds(ids.includes(plantId) ? ids.filter((x) => x !== plantId) : [...ids, plantId]);

  function pickDevice(id: string) {
    const d = devices?.find((x) => x.id === id);
    setF({ ...f, device_id: d?.id ?? (id || null), device_name: d?.name ?? (id ? f.device_name : null) });
  }

  async function submit() {
    if (!f.name.trim()) return setError('Укажите название');
    try {
      const data = { ...f, name: f.name.trim(), plant_ids: ids };
      if (lamp) await api.updateLamp(lamp.id, data);
      else await api.createLamp(data);
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сохранить');
    }
  }

  // Не <form>: блок живёт внутри формы настроек, вложенные формы запрещены
  return (
    <div className="fert-form">
      <label className="span-2">
        Название
        <input className="input" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Лампа цитрусы" />
      </label>
      <label className="span-2">
        Розетка
        <select className="select" value={f.device_id ?? ''} onChange={(e) => pickDevice(e.target.value)}>
          <option value="">Без розетки — только учёт часов</option>
          {f.device_id && !devices?.some((d) => d.id === f.device_id) && (
            <option value={f.device_id}>{f.device_name ?? f.device_id}</option>
          )}
          {devices?.map((d) => (
            <option key={d.id} value={d.id}>{d.name}{d.room ? ` — ${d.room}` : ''}</option>
          ))}
        </select>
      </label>
      {devicesError && <p className="field__hint span-2">{devicesError}</p>}
      <div className="span-2">
        <Segmented label="Режим" value={f.mode} options={MODES} onChange={(mode) => setF({ ...f, mode })} />
        <p className="field__hint" style={{ marginTop: 6 }}>{MODE_HINT[f.mode]}</p>
      </div>
      {f.mode === 'auto' && (
        <>
          <label>
            Утром не раньше
            <input className="input" type="time" value={f.morning_not_before} onChange={(e) => setF({ ...f, morning_not_before: e.target.value })} />
          </label>
          <label>
            Вечером не позже
            <input className="input" type="time" value={f.evening_not_after} onChange={(e) => setF({ ...f, evening_not_after: e.target.value })} />
          </label>
        </>
      )}
      {f.mode === 'schedule' &&
        (lamp?.mode === 'schedule' ? (
          <div className="span-2"><LampScheduleEditor lampId={lamp.id} initial={lamp.schedule} /></div>
        ) : (
          <p className="field__hint span-2">Сохраните лампу — после этого здесь появится расписание.</p>
        ))}
      <fieldset className="span-2 lamp-plants">
        <legend>Растения под лампой</legend>
        {plants.length === 0 && <p className="field__hint">Растений пока нет</p>}
        {plants.map((p) => {
          const other = otherLamp(p.id);
          return (
            <label className="check" key={p.id}>
              <input type="checkbox" checked={ids.includes(p.id)} onChange={() => toggleId(p.id)} />
              <span>
                {p.name}
                {other && !ids.includes(p.id) && <span className="field__hint"> — сейчас: {other.name}</span>}
              </span>
            </label>
          );
        })}
      </fieldset>
      {error && <p className="form-error span-2" role="alert">{error}</p>}
      <div className="btn-row span-2">
        <button className="btn btn--sm btn--primary" type="button" onClick={submit}>Сохранить</button>
        <button className="btn btn--sm" type="button" onClick={onCancel}>Отмена</button>
      </div>
    </div>
  );
}
