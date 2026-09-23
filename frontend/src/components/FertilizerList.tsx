import { useState } from 'react';
import { api } from '../api';
import { fmtNumber } from '../format';
import type { Fertilizer, FertilizerFields } from '../types';
import { useAsync } from '../useAsync';
import { useToast } from './toast';

const EMPTY: FertilizerFields = {
  name: '',
  npk: '',
  root_dose_ml_per_l: null,
  foliar_dose_ml_per_l: null,
  interval_days_active_season: 14,
  interval_days_dormant_season: 30,
};

function meta(f: Fertilizer): string {
  const parts = [
    f.npk && `NPK ${f.npk}`,
    f.root_dose_ml_per_l != null && `корень ${fmtNumber(f.root_dose_ml_per_l)} мл/л`,
    f.foliar_dose_ml_per_l != null && `лист ${fmtNumber(f.foliar_dose_ml_per_l)} мл/л`,
    `рост: раз в ${f.interval_days_active_season} д`,
    f.interval_days_dormant_season ? `покой: раз в ${f.interval_days_dormant_season} д` : 'в покое не удобрять',
  ];
  return parts.filter(Boolean).join(' · ');
}

/** CRUD типов удобрений. Сохраняется сразу, отдельно от формы настроек. */
export function FertilizerList() {
  const toast = useToast();
  const { data: list, reload } = useAsync(() => api.fertilizers(), []);
  const [editing, setEditing] = useState<number | 'new' | null>(null);

  async function remove(f: Fertilizer) {
    if (!confirm(`Удалить «${f.name}»? Записи подкормок останутся в истории.`)) return;
    try {
      await api.deleteFertilizer(f.id);
      reload();
    } catch {
      toast('Не удалось удалить');
    }
  }

  return (
    <div className="form-group">
      {list?.length === 0 && editing !== 'new' && <p className="empty">Удобрений пока нет</p>}
      {list?.map((f) =>
        editing === f.id ? (
          <FertilizerForm
            key={f.id}
            initial={f}
            onCancel={() => setEditing(null)}
            onSave={async (data) => { await api.updateFertilizer(f.id, data); setEditing(null); reload(); }}
          />
        ) : (
          <div className="fert-item" key={f.id}>
            <div className="fert-item__text">
              <div className="field__label">{f.name}</div>
              <div className="fert-item__meta">{meta(f)}</div>
            </div>
            <button className="btn btn--sm" type="button" onClick={() => setEditing(f.id)}>Изменить</button>
            <button className="btn btn--sm btn--danger" type="button" aria-label={`Удалить ${f.name}`} onClick={() => remove(f)}>✕</button>
          </div>
        ),
      )}
      {editing === 'new' ? (
        <FertilizerForm
          initial={EMPTY}
          onCancel={() => setEditing(null)}
          onSave={async (data) => { await api.createFertilizer(data); setEditing(null); reload(); }}
        />
      ) : (
        <div className="fert-item">
          <button className="btn btn--sm btn--ghost" type="button" onClick={() => setEditing('new')}>+ Добавить удобрение</button>
        </div>
      )}
    </div>
  );
}

const toStr = (n: number | null) => (n == null ? '' : String(n).replace('.', ','));
const toNum = (s: string) => {
  const n = Number(s.replace(',', '.'));
  return s.trim() === '' || Number.isNaN(n) ? null : n;
};

function FertilizerForm({
  initial,
  onSave,
  onCancel,
}: {
  initial: FertilizerFields;
  onSave: (data: FertilizerFields) => Promise<void>;
  onCancel: () => void;
}) {
  const [f, setF] = useState({
    name: initial.name,
    npk: initial.npk,
    root: toStr(initial.root_dose_ml_per_l),
    foliar: toStr(initial.foliar_dose_ml_per_l),
    active: toStr(initial.interval_days_active_season),
    dormant: toStr(initial.interval_days_dormant_season),
  });
  const [error, setError] = useState<string | null>(null);
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });

  async function submit() {
    const active = toNum(f.active);
    if (!f.name.trim()) return setError('Укажите название');
    if (!active || active < 1) return setError('Интервал в сезон роста — целое число дней от 1');
    try {
      await onSave({
        name: f.name.trim(),
        npk: f.npk.trim(),
        root_dose_ml_per_l: toNum(f.root),
        foliar_dose_ml_per_l: toNum(f.foliar),
        interval_days_active_season: Math.round(active),
        interval_days_dormant_season: toNum(f.dormant) ? Math.round(toNum(f.dormant)!) : null,
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сохранить');
    }
  }

  // Не <form>: блок живёт внутри формы настроек, вложенные формы запрещены
  return (
    <div className="fert-form" onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit(); } }}>
      <label className="span-2">Название<input className="input" value={f.name} onChange={set('name')} placeholder="Bona Forte" /></label>
      <label>NPK<input className="input" value={f.npk} onChange={set('npk')} placeholder="7-3-6" /></label>
      <span />
      <label>Под корень, мл/л<input className="input" inputMode="decimal" value={f.root} onChange={set('root')} /></label>
      <label>По листу, мл/л<input className="input" inputMode="decimal" value={f.foliar} onChange={set('foliar')} /></label>
      <label>Рост: раз в N дней<input className="input" inputMode="numeric" value={f.active} onChange={set('active')} /></label>
      <label>Покой: раз в N дней<input className="input" inputMode="numeric" value={f.dormant} onChange={set('dormant')} placeholder="не удобрять" /></label>
      {error && <p className="form-error span-2" role="alert">{error}</p>}
      <div className="btn-row span-2">
        <button className="btn btn--sm btn--primary" type="button" onClick={submit}>Сохранить</button>
        <button className="btn btn--sm" type="button" onClick={onCancel}>Отмена</button>
      </div>
    </div>
  );
}
