import { fmtHours, parseDate, shortDate } from '../format';
import type { WeekStat } from '../types';

const range = (w: WeekStat) => {
  const start = parseDate(w.week_start);
  const end = new Date(start);
  end.setDate(start.getDate() + 6);
  const f = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'short' });
  return `${start.getDate()}–${f.format(end)}${w.is_current ? ', идёт сейчас' : ''}`;
};

/** Столбики по неделям (разметка макета: .chart__bars с --max, .bar__seg с --n) */
export function WeekChart({ weeks, mode }: { weeks: WeekStat[]; mode: 'events' | 'lamp' }) {
  const max =
    mode === 'events'
      ? Math.max(1, ...weeks.map((w) => w.waterings + w.feedings))
      : Math.max(1, ...weeks.map((w) => w.lamp_hours + w.sunshine_hours));

  const label =
    mode === 'events'
      ? `Поливы и подкормки по неделям: до ${Math.max(...weeks.map((w) => w.waterings))} поливов в неделю`
      : `Свет по неделям, солнце и лампа: до ${fmtHours(max)} ч в неделю`;

  return (
    <>
      <div className="chart__bars" style={{ '--max': max } as React.CSSProperties} role="img" aria-label={label}>
        {weeks.map((w) => (
          <div
            key={w.week_start}
            className={`bar${w.is_current ? ' is-current' : ''}`}
            title={
              mode === 'events'
                ? `${range(w)}: полив ${w.waterings}, подкормка ${w.feedings}`
                : `${range(w)}: солнце ${fmtHours(w.sunshine_hours)} ч, лампа ${fmtHours(w.lamp_hours)} ч`
            }
          >
            {mode === 'events' ? (
              <>
                <i className="bar__seg bar__seg--water" style={{ '--n': w.waterings } as React.CSSProperties} />
                <i className="bar__seg bar__seg--feed" style={{ '--n': w.feedings } as React.CSSProperties} />
              </>
            ) : (
              <>
                <i className="bar__seg bar__seg--sun" style={{ '--n': w.sunshine_hours } as React.CSSProperties} />
                <i className="bar__seg bar__seg--lamp" style={{ '--n': w.lamp_hours } as React.CSSProperties} />
              </>
            )}
          </div>
        ))}
      </div>
      <div className="chart__axis" aria-hidden="true">
        {weeks.map((w) => <span key={w.week_start}>{shortDate(w.week_start)}</span>)}
      </div>
    </>
  );
}
