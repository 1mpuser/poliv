/** Русские формы: plural(3, ['день','дня','дней']) → 'дня' */
export function plural(n: number, [one, few, many]: readonly string[]): string {
  const m10 = Math.abs(n) % 10;
  const m100 = Math.abs(n) % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return few;
  return many;
}

export const DAYS = ['день', 'дня', 'дней'] as const;
export const HOURS = ['час', 'часа', 'часов'] as const;
export const MONTHS = ['месяц', 'месяца', 'месяцев'] as const;

const num = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 });
export const fmtNumber = (n: number) => num.format(n);

/** 9 → «9», 2.53 → «2,5» */
export const fmtHours = (h: number) => num.format(Math.round(h * 10) / 10);

const dayMonth = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' });
const dayMonthYear = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
const time = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' });
const weekday = new Intl.DateTimeFormat('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' });

/** '2026-09-02' (дата без времени) → Date в полночь по локальному времени */
export function parseDate(d: string): Date {
  const [y, m, day] = d.split('-').map(Number);
  return new Date(y, m - 1, day);
}

export function fmtDate(d: Date | string): string {
  const date = typeof d === 'string' ? (d.length === 10 ? parseDate(d) : new Date(d)) : d;
  if (date.getFullYear() === new Date().getFullYear()) return dayMonth.format(date);
  return dayMonthYear.format(date).replace(/\s?г\.$/, ''); // «2 сентября 2027», без «г.»
}

export const fmtTime = (iso: string) => time.format(new Date(iso));

export function fmtToday(): string {
  const s = weekday.format(new Date());
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Заголовок дня в ленте: «Сегодня», «Вчера», «21 сентября» */
export function dayLabel(date: Date): string {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const d = new Date(date);
  d.setHours(0, 0, 0, 0);
  const diff = Math.round((today.getTime() - d.getTime()) / 86_400_000);
  if (diff === 0) return 'Сегодня';
  if (diff === 1) return 'Вчера';
  return fmtDate(d);
}

/** «3 недели назад», «2 месяца назад» */
export function ago(iso: string): string {
  const days = Math.floor((Date.now() - new Date(iso).getTime()) / 86_400_000);
  if (days <= 0) return 'сегодня';
  if (days === 1) return 'вчера';
  if (days < 14) return `${days} ${plural(days, DAYS)} назад`;
  if (days < 60) {
    const w = Math.floor(days / 7);
    return `${w} ${plural(w, ['неделю', 'недели', 'недель'])} назад`;
  }
  const m = Math.floor(days / 30);
  return `${m} ${plural(m, MONTHS)} назад`;
}

/** Дата в формате input[type=date] по локальному времени */
export function isoDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export const shortDate = (d: string) => {
  const date = parseDate(d);
  return `${date.getDate()}.${String(date.getMonth() + 1).padStart(2, '0')}`;
};

/** Миниатюра по виду растения */
export function thumbClass(p: { name: string; species: string }): string {
  const s = `${p.name} ${p.species}`.toLowerCase();
  if (s.includes('лайм') || s.includes('lime')) return 'thumb--lime';
  if (s.includes('лимон') || s.includes('lemon')) return 'thumb--lemon';
  return 'thumb--plant';
}
