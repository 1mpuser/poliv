import { useEffect, useId, useState } from 'react';
import { DAYS, HOURS, MONTHS } from '../format';
import type { Fertilizer, PlantFields } from '../types';
import { Field, Stepper, Switch } from './controls';

export const EMPTY_PLANT: PlantFields = {
  name: '',
  species: '',
  location: null,
  pot_size_l: null,
  notes: null,
  water_interval_days: 4,
  fertilizing_enabled: true,
  fertilizing_interval_days: null,
  light_target_hours: 13,
  repot_check_interval_months: 12,
  fertilizer_ids: [],
};

function TextField({
  label,
  value,
  onChange,
  placeholder,
  multiline,
  inputMode,
  required,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  multiline?: boolean;
  inputMode?: 'decimal';
  required?: boolean;
}) {
  const id = useId();
  return (
    <div className="field field--stack">
      <label className="field__label" htmlFor={id}>{label}</label>
      {multiline ? (
        <textarea id={id} className="textarea" value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
      ) : (
        <input
          id={id}
          className="input"
          value={value}
          placeholder={placeholder}
          inputMode={inputMode}
          required={required}
          maxLength={100}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </div>
  );
}

const orNull = (s: string) => (s.trim() === '' ? null : s);

const parseDecimal = (s: string) => {
  const n = Number(s.replace(',', '.'));
  return s.trim() === '' || Number.isNaN(n) ? null : n;
};
const fmtDecimal = (n: number | null) => (n == null ? '' : String(n).replace('.', ','));

/** Дробное число с запятой: сырая строка живёт локально, чтобы «2,» не превращалось в «2» */
function DecimalField({ label, value, onChange, placeholder }: {
  label: string;
  value: number | null;
  onChange: (v: number | null) => void;
  placeholder?: string;
}) {
  const [raw, setRaw] = useState(fmtDecimal(value));
  useEffect(() => {
    if (parseDecimal(raw) !== value) setRaw(fmtDecimal(value));
  }, [value]);
  return (
    <TextField
      label={label}
      inputMode="decimal"
      value={raw}
      placeholder={placeholder}
      onChange={(v) => { setRaw(v); onChange(parseDecimal(v)); }}
    />
  );
}

/** Целое число дней: пустое поле — null («как у удобрения») */
function IntField({ label, value, onChange, placeholder }: {
  label: string;
  value: number | null;
  onChange: (v: number | null) => void;
  placeholder?: string;
}) {
  const [raw, setRaw] = useState(value == null ? '' : String(value));
  useEffect(() => {
    const cur = raw.trim() === '' ? null : Number(raw);
    if (cur !== value) setRaw(value == null ? '' : String(value));
  }, [value]);
  return (
    <TextField
      label={label}
      inputMode="decimal"
      value={raw}
      placeholder={placeholder}
      onChange={(v) => {
        setRaw(v);
        if (v.trim() === '') onChange(null);
        else {
          const n = Number(v);
          onChange(Number.isNaN(n) ? null : Math.max(1, Math.round(n)));
        }
      }}
    />
  );
}

/** Поля растения: описание + настройки ухода. Контролируемая форма. */
export function PlantForm({ value, fertilizers = [], onChange }: {
  value: PlantFields;
  /** Типы удобрений учётки — для галочек выбора набора растения */
  fertilizers?: Fertilizer[];
  onChange: (v: PlantFields) => void;
}) {
  const set = <K extends keyof PlantFields>(key: K, v: PlantFields[K]) => onChange({ ...value, [key]: v });
  // старый ответ может отдать null — не даём includes/filter упасть
  const ids = value.fertilizer_ids ?? [];

  return (
    <>
      <div className="form-group">
        <TextField label="Название" value={value.name} onChange={(v) => set('name', v)} placeholder="Лимон" required />
        <TextField label="Вид" value={value.species} onChange={(v) => set('species', v)} placeholder="Лимон Мейера" />
        <TextField label="Где стоит" value={value.location ?? ''} onChange={(v) => set('location', orNull(v))} placeholder="Кухня, восточное окно" />
        <DecimalField label="Объём горшка, л" value={value.pot_size_l} onChange={(v) => set('pot_size_l', v)} placeholder="3" />
        <TextField label="Заметки" multiline value={value.notes ?? ''} onChange={(v) => set('notes', orNull(v))} />
      </div>

      <h2 className="section__title" style={{ marginTop: 20 }}>Уход</h2>
      <div className="form-group">
        <Field label="Проверка грунта" hint="Раз в столько дней проверять">
          {(id) => <Stepper labelledBy={id} min={1} max={30} units={DAYS} value={value.water_interval_days} onChange={(v) => set('water_interval_days', v)} />}
        </Field>
        <Field label="Норма света" hint="Солнце + лампа, часов в день">
          {(id) => <Stepper labelledBy={id} min={0} max={20} units={HOURS} value={value.light_target_hours} onChange={(v) => set('light_target_hours', v)} />}
        </Field>
        <Field label="Подкормка" hint="Удобрять сейчас">
          {(id) => <Switch labelledBy={id} checked={value.fertilizing_enabled} onChange={(v) => set('fertilizing_enabled', v)} />}
        </Field>
        <Field label="Проверка горшка" hint="Раз в столько месяцев после пересадки">
          {(id) => (
            <Stepper
              labelledBy={id}
              min={1}
              max={36}
              units={MONTHS}
              value={value.repot_check_interval_months}
              onChange={(v) => set('repot_check_interval_months', v)}
            />
          )}
        </Field>
      </div>

      <h2 className="section__title" style={{ marginTop: 20 }}>Подкормка</h2>
      <div className="form-group">
        <Field label="Удобрения" hint="Из этих подкармливать — чередуются по кругу">
          {(id) => (
            <div className="fert-checks" aria-labelledby={id}>
              {fertilizers.length === 0 ? (
                <span className="field__hint">Сначала добавьте удобрения в разделе «Удобрения» ниже.</span>
              ) : (
                fertilizers.map((f) => (
                  <label className="check" key={f.id}>
                    <input
                      type="checkbox"
                      checked={ids.includes(f.id)}
                      onChange={(e) =>
                        set(
                          'fertilizer_ids',
                          e.target.checked
                            ? [...ids, f.id]
                            : ids.filter((x) => x !== f.id),
                        )
                      }
                    />
                    <span className="check__name">{f.name}</span>
                  </label>
                ))
              )}
            </div>
          )}
        </Field>
        <IntField
          label="Подкармливать раз в (дней)"
          value={value.fertilizing_interval_days}
          placeholder="пусто — как у удобрения"
          onChange={(v) => set('fertilizing_interval_days', v)}
        />
      </div>
    </>
  );
}
