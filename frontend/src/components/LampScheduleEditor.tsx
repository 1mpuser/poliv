import { useEffect, useState } from 'react';
import { api } from '../api';
import type { ScheduleInterval } from '../types';
import { useToast } from './toast';

const hhmm = (t: string) => t.slice(0, 5);
const hours = (list: ScheduleInterval[]) =>
  list.reduce((sum, i) => {
    const [sh, sm] = i.start_time.split(':').map(Number);
    const [eh, em] = i.end_time.split(':').map(Number);
    return sum + Math.max(0, eh * 60 + em - (sh * 60 + sm)) / 60;
  }, 0);

/** Расписание программируемой розетки одной лампы. Сохраняется сразу, отдельно от формы настроек.
 *  plantId=null — общая лампа. */
export function LampScheduleEditor({ plantId, initial }: { plantId: number | null; initial: ScheduleInterval[] }) {
  const toast = useToast();
  const [rows, setRows] = useState<ScheduleInterval[]>(initial.map((i) => ({ start_time: hhmm(i.start_time), end_time: hhmm(i.end_time) })));
  const [saved, setSaved] = useState(JSON.stringify(rows));
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const next = initial.map((i) => ({ start_time: hhmm(i.start_time), end_time: hhmm(i.end_time) }));
    setRows(next);
    setSaved(JSON.stringify(next));
  }, [plantId, JSON.stringify(initial)]);

  const dirty = JSON.stringify(rows) !== saved;
  const set = (i: number, key: keyof ScheduleInterval, v: string) =>
    setRows(rows.map((r, j) => (j === i ? { ...r, [key]: v } : r)));

  async function save() {
    setBusy(true);
    try {
      const res = await api.setLampSchedule(plantId, rows.filter((r) => r.start_time && r.end_time));
      const next = res.map((i) => ({ start_time: hhmm(i.start_time), end_time: hhmm(i.end_time) }));
      setRows(next);
      setSaved(JSON.stringify(next));
      toast(next.length ? `Расписание сохранено: ${hours(next).toFixed(1).replace('.', ',')} ч в день` : 'Расписание убрано');
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сохранить расписание');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      {rows.length === 0 && <p className="field__hint">Расписания нет — лампа только вручную.</p>}
      {rows.map((r, i) => (
        <div className="sched-row" key={i}>
          <input className="input" type="time" aria-label="Включение" value={r.start_time} onChange={(e) => set(i, 'start_time', e.target.value)} />
          <span>—</span>
          <input className="input" type="time" aria-label="Выключение" value={r.end_time} onChange={(e) => set(i, 'end_time', e.target.value)} />
          <button className="btn btn--sm btn--danger" type="button" aria-label="Убрать интервал" onClick={() => setRows(rows.filter((_, j) => j !== i))}>✕</button>
        </div>
      ))}
      <div className="btn-row" style={{ marginTop: 8 }}>
        <button
          className="btn btn--sm btn--ghost"
          type="button"
          disabled={rows.length >= 8}
          onClick={() => setRows([...rows, rows.length ? { start_time: '18:00', end_time: '22:00' } : { start_time: '07:00', end_time: '10:00' }])}
        >
          + Интервал
        </button>
        {dirty && (
          <button className="btn btn--sm btn--primary" type="button" disabled={busy} onClick={save}>Сохранить расписание</button>
        )}
      </div>
    </div>
  );
}
