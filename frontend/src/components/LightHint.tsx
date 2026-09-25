import { fmtHours, fmtTime } from '../format';
import type { LightSummary, PlannedInterval } from '../types';
import { Icon } from './icons';

/** Склеить соседние интервалы (конец одной = начало другой): день и вечер идут слитно */
function mergeIntervals(planned: PlannedInterval[]): PlannedInterval[] {
  const sorted = [...planned].sort((a, b) => a.start.localeCompare(b.start));
  const out: PlannedInterval[] = [];
  for (const p of sorted) {
    const last = out[out.length - 1];
    if (last && last.end !== null && p.start <= last.end) {
      out[out.length - 1] = { start: last.start, end: p.end ?? last.end };
    } else {
      out.push(p);
    }
  }
  return out;
}

/** Строка под плитками: сколько света не хватает до нормы и когда включить лампу (или когда она досветит сама) */
export function LightHint({ light }: { light: LightSummary }) {
  const auto = light.lamp?.mode === 'auto' ? light.lamp : null;
  const plan = auto?.planned
    ? mergeIntervals(auto.planned).map((p) => `${fmtTime(p.start)}–${p.end ? fmtTime(p.end) : '…'}`).join(' и ')
    : undefined;
  let text: string;
  if (light.deficit_hours <= 0) {
    text = `Света хватает: ${fmtHours(light.total_hours)} ч при норме ${fmtHours(light.target_hours)}`;
    if (plan) text += ` — ${auto!.name} досветит ${plan}`;
  } else if (auto) {
    text = `Не хватает ${fmtHours(light.deficit_hours)} ч даже с досветкой`;
    if (plan) text += ` (${plan})`;
  } else if (light.suggestion_start && light.suggestion_end) {
    text = `Не хватает ${fmtHours(light.deficit_hours)} ч — лампа ${fmtTime(light.suggestion_start)}–${fmtTime(light.suggestion_end)}`;
    if (light.suggestion_until_midnight) text += ', и до полуночи не хватит';
  } else {
    text = `Сегодня не хватило ${fmtHours(light.deficit_hours)} ч света`;
  }
  if (light.natural_hours === null) text += '. Город не задан — солнце не учтено';
  return (
    <p className="plant-card__foot" data-kind="light" data-status={light.deficit_hours > 0 ? light.status : 'ok'}>
      <Icon name="lamp" />
      <span>{text}</span>
    </p>
  );
}
