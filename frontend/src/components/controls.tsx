import { useEffect, useId, useRef, type ReactNode } from 'react';
import { plural } from '../format';
import { Icon } from './icons';

/* ---------- Степпер (контракт макета: min/max/step/units, удержание — быстрая прокрутка) ---------- */
interface StepperProps {
  value: number;
  onChange: (v: number) => void;
  min?: number;
  max?: number;
  step?: number;
  units?: readonly string[];
  labelledBy?: string;
}

export function Stepper({ value, onChange, min = 0, max = 99, step = 1, units, labelledBy }: StepperProps) {
  const valueRef = useRef(value);
  valueRef.current = value;
  const timers = useRef<{ hold?: number; repeat?: number }>({});

  const change = (dir: number) => {
    const next = Math.min(max, Math.max(min, valueRef.current + dir * step));
    if (next === valueRef.current) return false;
    valueRef.current = next;
    onChange(next);
    return true;
  };
  const stop = () => {
    window.clearTimeout(timers.current.hold);
    window.clearInterval(timers.current.repeat);
  };
  useEffect(() => stop, []);

  const btn = (dir: 1 | -1) => (
    <button
      className="stepper__btn"
      type="button"
      aria-label={dir > 0 ? 'Больше' : 'Меньше'}
      disabled={dir > 0 ? value >= max : value <= min}
      onPointerDown={(e) => {
        if (e.button !== 0) return;
        change(dir);
        timers.current.hold = window.setTimeout(() => {
          timers.current.repeat = window.setInterval(() => { if (!change(dir)) stop(); }, 90);
        }, 400);
      }}
      onPointerUp={stop}
      onPointerLeave={stop}
      onPointerCancel={stop}
      // Клавиатура: Enter/Space (pointerdown не срабатывает)
      onClick={(e) => { if (e.detail === 0) change(dir); }}
    >
      <Icon name={dir > 0 ? 'plus' : 'minus'} />
    </button>
  );

  const unit = units ? plural(value, units) : '';
  return (
    <div className="stepper">
      {btn(-1)}
      <span
        className="stepper__value"
        role="spinbutton"
        tabIndex={0}
        aria-labelledby={labelledBy}
        aria-valuenow={value}
        aria-valuemin={min}
        aria-valuemax={max}
        aria-valuetext={`${value} ${unit}`.trim()}
        onKeyDown={(e) => {
          const map: Record<string, number> = { ArrowUp: 1, ArrowRight: 1, ArrowDown: -1, ArrowLeft: -1 };
          if (e.key in map) { e.preventDefault(); change(map[e.key]); }
          if (e.key === 'Home') { e.preventDefault(); onChange(min); }
          if (e.key === 'End') { e.preventDefault(); onChange(max); }
        }}
      >
        <span className="stepper__num">{value}</span>
        <span className="stepper__unit">{unit}</span>
      </span>
      {btn(1)}
    </div>
  );
}

/* ---------- Строка настройки: подпись + подсказка + контрол ---------- */
export function Field({ label, hint, children }: { label: string; hint?: string; children: (id: string) => ReactNode }) {
  const id = useId();
  return (
    <div className="field">
      <span className="field__text">
        <span className="field__label" id={id}>{label}</span>
        {hint && <span className="field__hint">{hint}</span>}
      </span>
      {children(id)}
    </div>
  );
}

export function Switch({ checked, onChange, labelledBy }: { checked: boolean; onChange: (v: boolean) => void; labelledBy?: string }) {
  return (
    <input
      className="switch"
      type="checkbox"
      role="switch"
      aria-labelledby={labelledBy}
      checked={checked}
      onChange={(e) => onChange(e.target.checked)}
    />
  );
}

/* ---------- Сегментированный переключатель ---------- */
export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  return (
    <div className="segmented" role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

/* ---------- Нижняя шторка на <dialog> ---------- */
export function Sheet({ open, onClose, children }: { open: boolean; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      className="sheet"
      onClose={onClose}
      onClick={(e) => { if (e.target === ref.current) onClose(); }}
    >
      {open && <div className="sheet__body">{children}</div>}
    </dialog>
  );
}
