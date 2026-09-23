import { Link } from 'react-router';
import { api } from '../api';
import { Icon } from '../components/icons';
import { PlantCard } from '../components/PlantCard';
import { useToast } from '../components/toast';
import { fmtToday } from '../format';
import type { PlantSummary, Status } from '../types';
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
    () => Promise.all([api.summaries(), api.fertilizers()]),
    [],
  );
  const [summaries, fertilizers] = data ?? [undefined, []];
  const sharedOn = summaries?.[0]?.lamp.shared_is_on ?? false;

  async function toggleShared() {
    try {
      const { is_on, session } = await api.toggleLamp(null);
      reload();
      toast(is_on ? 'Общая лампа включена' : 'Общая лампа выключена', async () => {
        if (is_on) await api.deleteLamp(session.id);
        else await api.reopenLamp(session.id);
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

      {summaries && summaries.length > 0 && (
        <div className="toolbar">
          <button className="chip" type="button" aria-pressed={sharedOn} data-on={sharedOn} onClick={toggleShared}>
            <Icon name="lamp" />
            {sharedOn ? 'Общая лампа горит' : 'Общая лампа'}
          </button>
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
