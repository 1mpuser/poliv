import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { api } from '../api';
import { Icon } from '../components/icons';
import { EMPTY_PLANT, PlantForm } from '../components/PlantForm';
import { useToast } from '../components/toast';
import { useAsync } from '../useAsync';

export function NewPlant() {
  const navigate = useNavigate();
  const toast = useToast();
  const { data: fertilizers } = useAsync(() => api.fertilizers(), []);
  const [draft, setDraft] = useState(EMPTY_PLANT);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  return (
    <>
      <header className="page-head">
        <Link className="icon-btn" to="/" aria-label="Назад к растениям"><Icon name="back" /></Link>
        <div className="page-head__grow">
          <h1 className="page-head__title">Новое растение</h1>
        </div>
      </header>

      <form
        noValidate
        onSubmit={async (e) => {
          e.preventDefault();
          if (!draft.name.trim()) {
            setError('Укажите название');
            return;
          }
          setBusy(true);
          setError(null);
          try {
            const plant = await api.createPlant({ ...draft, name: draft.name.trim() });
            toast(`${plant.name} добавлен(а)`);
            navigate(`/plants/${plant.id}`, { replace: true });
          } catch (err) {
            setError(err instanceof Error ? err.message : 'Не удалось сохранить');
          } finally {
            setBusy(false);
          }
        }}
      >
        <PlantForm value={draft} fertilizers={fertilizers ?? []} onChange={setDraft} />
        {error && <p className="form-error" role="alert" style={{ marginTop: 12 }}>{error}</p>}
        <div className="save-bar">
          <button className="btn-primary" type="submit" disabled={busy}>Добавить</button>
        </div>
      </form>
    </>
  );
}
