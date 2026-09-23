import { useState } from 'react';
import { api } from '../api';
import { useMe } from '../me';
import { useToast } from './toast';

/** Почта учётки, смена своего пароля, выход. Не <form>: блок внутри формы настроек. */
export function AccountSection({ onLogout }: { onLogout: () => void }) {
  const me = useMe();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit() {
    setError(null);
    if (next.length < 8) return setError('Новый пароль — не короче 8 символов');
    setBusy(true);
    try {
      await api.changePassword(current, next);
      setOpen(false);
      setCurrent('');
      setNext('');
      toast('Пароль изменён. На других устройствах нужно войти заново.');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сменить пароль');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="form-group">
      <div className="field">
        <span className="field__text">
          <span className="field__label">{me.email}</span>
          <span className="field__hint">{me.is_admin ? 'Администратор' : 'Учётка'}</span>
        </span>
        <button className="btn btn--sm" type="button" onClick={onLogout}>Выйти</button>
      </div>
      {open ? (
        <div className="fert-form" onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit(); } }}>
          <label className="span-2">Текущий пароль
            <input className="input" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
          </label>
          <label className="span-2">Новый пароль
            <input className="input" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
          </label>
          {error && <p className="form-error span-2" role="alert">{error}</p>}
          <div className="btn-row span-2">
            <button className="btn btn--sm btn--primary" type="button" disabled={busy} onClick={submit}>Сменить пароль</button>
            <button className="btn btn--sm" type="button" onClick={() => { setOpen(false); setError(null); }}>Отмена</button>
          </div>
        </div>
      ) : (
        <div className="field">
          <button className="btn btn--sm btn--ghost" type="button" onClick={() => setOpen(true)}>Сменить пароль</button>
        </div>
      )}
    </div>
  );
}
