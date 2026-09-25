import { Link } from 'react-router';
import { api } from '../api';
import { Icon } from '../components/icons';
import { PlantCard } from '../components/PlantCard';
import { useToast } from '../components/toast';
import { fmtToday } from '../format';
import type { Lamp, PlantSummary, Status } from '../types';
import { useAsync } from '../useAsync';

/** Сводка «что сделать сейчас» из статусов, посчитанных бэкендом */
function todo(list: PlantSummary[]): { status: Status; text: string } {
  const lines: string[] = [];
  let status: Status = 'ok';
  for (const s of list) {
    const parts: string[] = [];
    if (s.water.status === 'late') parts.push('пора полить');
    else if (s.water.status === 'soon') parts.push('скоро полив');
    if (s.feed.status === 'late') parts.push('пора подкормить');
    else if (s.feed.status === 'soon') parts.push('скоро подкормка');
    if (s.repot.status === 'late') parts.push('пора проверить горшок');
    if (parts.length === 0) continue;
    lines.push(`${s.plant.name}: ${parts.join(', ')}.`);
    if ([s.water.status, s.feed.status, s.repot.status].includes('late')) status = 'late';
    else if (status !== 'late') status = 'soon';
  }
  return { status, text: lines.length ? lines.join(' ') : 'Все растения в порядке' };
}

export function Dashboard() {
  const toast = useToast();
  const { data, error, reload } = useAsync(
    () => Promise.all([api.summaries(), api.fertilizers(), api.lamps()]),
    [],
  );
  const [summaries, fertilizers, lamps] = data ?? [undefined, [], []];

  async function toggle(l: Lamp) {
    try {
      const { is_on, session, previous_ended_at, plug_error } = await api.toggleLampById(l.id);
      reload();
      const base = `${l.name}: ${is_on ? 'включена' : 'выключена'}`;
      toast(plug_error ? `${base}, но розетка не ответила: ${plug_error}` : base, async () => {
        if (is_on) await api.deleteLampSession(session.id);
        else await api.restoreLampEnd(session.id, previous_ended_at);
        reload();
      });
    } catch {
      toast('Не удалось переключить лампу');
    }
  }

  const summary = summaries && todo(summaries);

  return (
    <>
      <header className="page-head">
        <img className="logo" src="/icon.svg" alt="" width={48} height={48} />
        <div className="page-head__grow">
          <h1 className="page-head__title">Мои растения</h1>
          <p className="page-head__sub">{fmtToday()}</p>
        </div>
      </header>

      {error && <p className="todo" data-status="late"><span className="todo__dot" />{error}</p>}

      {summary && summaries!.length > 0 && (
        <p className="todo" data-status={summary.status}>
          <span className="todo__dot" />
          {summary.text}
        </p>
      )}

      {lamps.length > 0 && (
        <div className="toolbar">
          {lamps.map((l) => (
            <button className="chip" type="button" key={l.id} aria-pressed={l.is_on} data-on={l.is_on} onClick={() => toggle(l)}>
              <Icon name="lamp" />
              {l.is_on ? `${l.name} горит` : l.name}
            </button>
          ))}
        </div>
      )}

      <section className="plant-grid" aria-label="Растения">
        {summaries?.map((s) => (
          <PlantCard key={s.plant.id} s={s} fertilizers={fertilizers} onChanged={reload} />
        ))}
        {summaries && (
          <Link className="add-card" to="/plants/new">
            <Icon name="plus" />
            Добавить растение
          </Link>
        )}
      </section>
    </>
  );
}
