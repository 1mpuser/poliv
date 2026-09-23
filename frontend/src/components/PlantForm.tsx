import { useEffect, useId, useState } from 'react';
import { DAYS, HOURS, MONTHS } from '../format';
import type { PlantFields } from '../types';
import { Field, Stepper, Switch } from './controls';

export const EMPTY_PLANT: PlantFields = {
  name: '',
  species: '',
  location: null,
  pot_size_l: null,
  notes: null,
  water_interval_days: 4,
  fertilizing_enabled: true,
  lamp_hours_per_day: 12,
  repot_check_interval_months: 12,
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

/** Поля растения: описание + настройки ухода. Контролируемая форма. */
export function PlantForm({ value, onChange }: { value: PlantFields; onChange: (v: PlantFields) => void }) {
  const set = <K extends keyof PlantFields>(key: K, v: PlantFields[K]) => onChange({ ...value, [key]: v });

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
        <Field label="Полив" hint="Раз в столько дней">
          {(id) => <Stepper labelledBy={id} min={1} max={30} units={DAYS} value={value.water_interval_days} onChange={(v) => set('water_interval_days', v)} />}
        </Field>
        <Field label="Досветка" hint="Часов в день">
          {(id) => <Stepper labelledBy={id} min={0} max={16} units={HOURS} value={value.lamp_hours_per_day} onChange={(v) => set('lamp_hours_per_day', v)} />}
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
    </>
  );
}
