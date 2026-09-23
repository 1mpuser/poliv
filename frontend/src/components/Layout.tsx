import { Link, Outlet, useLocation } from 'react-router';
import { useMe } from '../me';
import { Icon } from './icons';

export function Layout() {
  const { is_admin } = useMe();
  const path = useLocation().pathname;
  const section = path.startsWith('/settings') ? 'settings' : path.startsWith('/admin') ? 'admin' : 'plants';
  const current = (s: string) => (section === s ? 'page' : undefined);

  return (
    <>
      <main className="wrap">
        <Outlet />
      </main>
      <nav className="tabbar" aria-label="Разделы">
        <div className="tabbar__inner">
          <Link to="/" aria-current={current('plants')}>
            <Icon name="plants" />Растения
          </Link>
          <Link to="/settings" aria-current={current('settings')}>
            <Icon name="settings" />Настройки
          </Link>
          {is_admin && (
            <Link to="/admin" aria-current={current('admin')}>
              <Icon name="users" />Админка
            </Link>
          )}
        </div>
      </nav>
    </>
  );
}
