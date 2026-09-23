import { Link, Outlet, useLocation } from 'react-router';
import { Icon } from './icons';

export function Layout() {
  const inSettings = useLocation().pathname.startsWith('/settings');
  return (
    <>
      <main className="wrap">
        <Outlet />
      </main>
      <nav className="tabbar" aria-label="Разделы">
        <div className="tabbar__inner">
          <Link to="/" aria-current={inSettings ? undefined : 'page'}>
            <Icon name="plants" />Растения
          </Link>
          <Link to="/settings" aria-current={inSettings ? 'page' : undefined}>
            <Icon name="settings" />Настройки
          </Link>
        </div>
      </nav>
    </>
  );
}
