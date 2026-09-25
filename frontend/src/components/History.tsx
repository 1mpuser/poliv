import { useState } from 'react';
import { api } from '../api';
import { dayLabel, fmtHours, fmtNumber, fmtTime } from '../format';
import type { EventType, HistoryEvent } from '../types';
import { useAsync } from '../useAsync';
import { Icon } from './icons';

const FILTERS: { value: EventType | 'all'; label: string }[] = [
  { value: 'all', label: 'Все' },
  { value: 'water', label: 'Полив' },
  { value: 'feed', label: 'Подкормка' },
  { value: 'lamp', label: 'Лампа' },
  { value: 'repot', label: 'Пересадка' },
];

function describe(e: HistoryEvent): { title: string; note: string } {
  const extra = e.note ? [e.note] : [];
  switch (e.type) {
    case 'water':
      return { title: 'Полив', note: extra.join('') };
    case 'feed':
      return {
        title: 'Подкормка',
        note: [e.fertilizer_name ?? 'удобрение удалено', e.method === 'foliar' ? 'по листу' : 'под корень', ...extra].join(', '),
      };
    case 'lamp':
      return {
        title: e.lamp_name ?? 'Лампа',
        note: e.ended_at
          ? `${fmtTime(e.at)}–${fmtTime(e.ended_at)}, ${fmtHours(e.hours ?? 0)} ч`
          : `Горит с ${fmtTime(e.at)}`,
      };
    case 'repot': {
      const sizes =
        e.pot_size_before != null && e.pot_size_after != null
          ? `Из горшка ${fmtNumber(e.pot_size_before)} л в ${fmtNumber(e.pot_size_after)} л`
          : e.pot_size_after != null ? `В горшок ${fmtNumber(e.pot_size_after)} л` : '';
      return { title: 'Пересадка', note: [sizes, ...extra].filter(Boolean).join(', ') };
    }
  }
}

/** Лента всех событий растения с фильтром по типу и диапазону дат. version — повод перезапросить. */
export function History({ plantId, version }: { plantId: number; version: number }) {
  const [filter, setFilter] = useState<EventType | 'all'>('all');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const { data: events, error } = useAsync(
    () => api.history(plantId, { types: filter === 'all' ? undefined : [filter], from, to }),
    [plantId, filter, from, to, version],
  );

  const items: React.ReactNode[] = [];
  let lastDay = '';
  for (const e of events ?? []) {
    const d = new Date(e.at);
    const day = d.toDateString();
    if (day !== lastDay) {
      items.push(<li key={`d-${day}`} className="tl-day">{dayLabel(d)}</li>);
      lastDay = day;
    }
    const { title, note } = describe(e);
    items.push(
      <li key={`${e.type}-${e.id}`} className="tl-item" data-type={e.type}>
        <span className="tl-ico"><Icon name={e.type} /></span>
        <span>
          <span className="tl-title">{title}</span>
          {note && <><br /><span className="tl-note">{note}</span></>}
        </span>
        <time className="tl-time" dateTime={e.at}>{fmtTime(e.at)}</time>
      </li>,
    );
  }

  return (
    <section className="section">
      <h2 className="section__title">История</h2>
      <div className="chips" role="toolbar" aria-label="Фильтр событий">
        {FILTERS.map((f) => (
          <button key={f.value} className="chip" type="button" aria-pressed={filter === f.value} onClick={() => setFilter(f.value)}>
            {f.label}
          </button>
        ))}
      </div>
      <div className="date-range">
        <label>С
          <input className="input" type="date" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} />
        </label>
        <label>По
          <input className="input" type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} />
        </label>
        <button className="btn btn--sm btn--ghost" type="button" disabled={!from && !to} onClick={() => { setFrom(''); setTo(''); }}>
          Сбросить
        </button>
      </div>
      <div className="panel">
        {error && <p className="form-error">{error}</p>}
        {events && events.length === 0 && <p className="empty">Событий нет</p>}
        <ol className="timeline">{items}</ol>
      </div>
    </section>
  );
}
