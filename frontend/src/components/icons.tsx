/** Спрайт иконок из макета — рендерится один раз в корне, дальше <Icon name="water" /> */
export function IconSprite() {
  return (
    <svg width="0" height="0" style={{ position: 'absolute' }} aria-hidden="true">
      <symbol id="i-water" viewBox="0 0 24 24"><path d="M12 3s6 6.5 6 11a6 6 0 0 1-12 0c0-4.5 6-11 6-11z" /></symbol>
      <symbol id="i-feed" viewBox="0 0 24 24"><path d="M12 21v-9" /><path d="M12 12c0-4 3-6.5 7-6.5 0 4-3 6.5-7 6.5z" /><path d="M12 14.5c0-3-2.5-5-6-5 0 3 2.5 5 6 5z" /></symbol>
      <symbol id="i-lamp" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></symbol>
      <symbol id="i-repot" viewBox="0 0 24 24"><path d="M5 11h14l-2 9H7z" /><path d="M12 11V6" /><path d="M12 7.5c-2 0-3.5-1.2-3.5-3.5 2 0 3.5 1.2 3.5 3.5zM12 6.5c0-2.2 1.3-3.5 3.5-3.5 0 2.2-1.3 3.5-3.5 3.5z" /></symbol>
      <symbol id="i-leaf" viewBox="0 0 24 24"><path d="M4 20C4 10 10 4 20 4c0 10-6 16-16 16z" /><path d="M4 20 14 10" /></symbol>
      <symbol id="i-chev" viewBox="0 0 24 24"><path d="M9 6l6 6-6 6" /></symbol>
      <symbol id="i-back" viewBox="0 0 24 24"><path d="M15 6l-6 6 6 6" /></symbol>
      <symbol id="i-plus" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" /></symbol>
      <symbol id="i-minus" viewBox="0 0 24 24"><path d="M5 12h14" /></symbol>
      <symbol id="i-plants" viewBox="0 0 24 24"><rect x="4" y="4" width="7" height="7" rx="2" /><rect x="13" y="4" width="7" height="7" rx="2" /><rect x="4" y="13" width="7" height="7" rx="2" /><rect x="13" y="13" width="7" height="7" rx="2" /></symbol>
      <symbol id="i-users" viewBox="0 0 24 24"><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20c0-3.6 2.9-6 6.5-6s6.5 2.4 6.5 6" /><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8M18 14.3c2.2.7 3.5 2.8 3.5 5.7" /></symbol>
      <symbol id="i-settings" viewBox="0 0 24 24"><path d="M4 7h9M19 7h1M4 17h3M11 17h9" /><circle cx="16" cy="7" r="2.5" /><circle cx="9" cy="17" r="2.5" /></symbol>
    </svg>
  );
}

export type IconName =
  | 'water' | 'feed' | 'lamp' | 'repot' | 'leaf' | 'chev' | 'back' | 'plus' | 'minus' | 'plants' | 'settings' | 'users';

export function Icon({ name, className = 'i' }: { name: IconName; className?: string }) {
  return (
    <svg className={className} aria-hidden="true">
      <use href={`#i-${name}`} />
    </svg>
  );
}

export function Thumb({ className, large }: { className: string; large?: boolean }) {
  return (
    <span className={`thumb ${className}${large ? ' thumb--lg' : ''}`} aria-hidden="true">
      <svg><use href="#i-leaf" /></svg>
    </span>
  );
}
