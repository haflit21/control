import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import type { CSSProperties, ReactNode } from 'react';
import type { Alert, Person } from '@control/model';
import { Icon, Pict, Tag, Dot, Pwr, cx } from './primitives';
import type { IconName } from './sprite';

/** Ambient placeholder behind a hero or room tile while no image exists. */
export function Ambience({ variant }: { variant?: 'salon' | 'bureau' | 'chambre' | 'cuisine' | 'exterieur' | string }) {
  const known = ['salon', 'bureau', 'chambre', 'cuisine', 'exterieur'];
  const v = variant && known.includes(variant) ? variant : undefined;
  return <div className={cx('ph', v && `-${v}`)} aria-hidden="true" />;
}

/** A cover photo that falls back to the drawn ambience when the file cannot load. */
export function Photo({ src, ambience }: { src: string; ambience?: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  if (failed) return <Ambience variant={ambience} />;
  return <img className="hero-img" src={src} alt="" onError={() => setFailed(true)} />;
}

/** Photo / camera hero card. image: a URL (snapshot) or undefined for the ambience. */
export function Hero({ image, ambience, wide, cinema, live, chips, title, sub, pict, center, state, href, onClick, className, overlay }: {
  image?: string; ambience?: string; wide?: boolean; cinema?: boolean; live?: boolean; chips?: ReactNode; title: string; sub?: string; pict?: IconName | string;
  center?: { icon: IconName | string; text: string }; state?: 'private' | 'offline' | 'loading'; href?: string; onClick?: () => void; className?: string; overlay?: ReactNode;
}) {
  const inner = (
    <>
      {image ? <Photo src={image} ambience={ambience} /> : <Ambience variant={ambience} />}
      {(live || chips) && (
        <div className="hero-top">
          {live ? <Tag live>LIVE</Tag> : <span />}
          {chips && <div className="chip-row">{chips}</div>}
        </div>
      )}
      {center && <div className="hero-center"><Pict icon={center.icon} size="l" className="-on-photo" />{center.text}</div>}
      {overlay}
      <div className={cx('hero-bot', !!overlay && '-right')}>
        {pict && !overlay && <Pict icon={pict} />}
        <span className="hero-body"><span className="hero-t">{title}</span>{sub && <span className="hero-s">{sub}</span>}</span>
        {(href || onClick) && <Icon name="expand" style={{ color: 'rgba(255,255,255,.7)' }} />}
      </div>
    </>
  );
  const cls = cx('hero', wide && '-wide', cinema && '-cinema', state && `is-${state}`, !!image && !live && 'has-img', className);
  if (href) return <a className={cls} href={href} onClick={onClick}>{inner}</a>;
  if (onClick) return <button type="button" className={cls} onClick={onClick}>{inner}</button>;
  return <div className={cls}>{inner}</div>;
}

/** Device card (portrait): drawn object or image, glass label, power button. */
export function DevCard({ title, sub, on, icon, object, image, tint, href, onClick, offline, onPower, pending }: { title: string; sub?: string; on: boolean; icon?: IconName | string; object?: 'lamp' | 'speaker' | 'shade'; image?: string; tint?: string; href?: string; onClick?: () => void; offline?: boolean; onPower?: () => void; pending?: boolean }) {
  const inner = (
    <>
      {image ? <Photo src={image} /> : <Ambience />}
      {image && tint && <div className="dev-tint" aria-hidden="true" />}
      {!image && !offline && <div className={cx('obj', object === 'speaker' && '-speaker', object === 'shade' && '-shade')} aria-hidden="true" />}
      {offline && <div className="hero-center"><Pict icon="wifi-off" size="l" />Injoignable</div>}
      <div className="hero-bot"><span className="hero-body"><span className="hero-t">{title}</span>{sub && <span className="hero-s">{sub}</span>}</span></div>
      {!offline && <Pwr on={on} icon={icon ?? 'power'} label={on ? 'Allumé' : 'Éteint'} onClick={onPower} pending={pending} />}
    </>
  );
  const cls = cx('dev', !on && 'is-off', offline && 'is-offline', !!image && 'has-img', !!tint && 'has-tint', pending && 'is-pending', !!onClick && !href && 'is-clickable');
  const style = tint ? ({ ['--tint' as string]: tint } as CSSProperties) : undefined;
  if (href) return <a className={cls} style={style} href={href} onClick={onClick}>{inner}</a>;
  if (onClick) return <div className={cls} style={style}><button type="button" className="dev-open" aria-label={`${title} · réglages`} onClick={onClick} />{inner}</div>;
  return <div className={cls} style={style}>{inner}</div>;
}

export function Tile({ icon, value, unit, label, tone, glass, children, style }: { icon?: IconName | string; value?: ReactNode; unit?: string; label: string; tone?: 'peach' | 'blue' | 'mint' | 'sand' | 'night' | 'day'; glass?: boolean; children?: ReactNode; style?: CSSProperties }) {
  return (
    <div className={cx('tile', tone && tone !== 'peach' && `-${tone}`, glass && '-glass', !!children && '-wx')} style={style}>
      {icon && <Pict icon={icon} size="s" />}
      <div><div className="tile-v">{value}{unit && <small>{unit}</small>}</div><div className="tile-l">{label}</div></div>
      {children}
    </div>
  );
}

export function Player({ title, artist, playing, volume, progress, cover, onToggle, pending }: { title: string; artist?: string; playing: boolean; volume?: number; progress?: { pos: string; len: string; pct: number } | 'live'; cover?: string; onToggle?: () => void; pending?: boolean }) {
  const [coverFailed, setCoverFailed] = useState(false);
  useEffect(() => setCoverFailed(false), [cover]);
  return (
    <div className={cx('player', !playing && 'is-idle')}>
      {cover && !coverFailed ? <img className="player-cover" src={cover} alt="" onError={() => setCoverFailed(true)} /> : <span className="player-cover" aria-hidden="true" />}
      <span className="player-body">
        <span className="player-t">{title}</span>
        {artist && <span className="player-a">{artist}{volume != null && ` · ${volume} %`}</span>}
        {progress === 'live' && <span className="player-prog"><span>direct</span><span className="gauge-track"><span className="gauge-fill" style={{ ['--v' as string]: '100%' }} /></span><span /></span>}
        {progress && progress !== 'live' && <span className="player-prog"><span>{progress.pos}</span><span className="gauge-track"><span className="gauge-fill" style={{ ['--v' as string]: `${progress.pct}%` }} /></span><span>{progress.len}</span></span>}
      </span>
      <Pwr on={playing} icon={playing ? 'pause' : 'play'} label={playing ? 'En lecture' : 'En pause'} onClick={onToggle} pending={pending} />
    </div>
  );
}

export function Seg({ items, active, onSelect, renderLink }: { items: Array<{ id: string; label: string; badge?: string }>; active: string; onSelect?: (id: string) => void; renderLink?: (item: { id: string; label: string }, cls: string, children: ReactNode) => ReactNode }) {
  return (
    <nav className="seg" role="tablist">
      {items.map((it) => {
        const cls = cx(it.id === active && 'is-active');
        const children = <>{it.label}{it.badge && <span className="seg-n">{it.badge}</span>}</>;
        if (renderLink) return <span key={it.id} style={{ display: 'contents' }}>{renderLink(it, cls, children)}</span>;
        return <button key={it.id} type="button" role="tab" aria-selected={it.id === active} className={cls} onClick={() => onSelect?.(it.id)}>{children}</button>;
      })}
    </nav>
  );
}

export function WhoPill({ people }: { people: Person[] }) {
  const home = people.filter((p) => p.home).length;
  const txt = home === 0 ? 'Personne' : `${home} / ${people.length}`;
  return (
    <span className={cx('who-pill', home > 0 && 'is-home')}>
      <span className="who-stack" aria-hidden="true">
        {people.map((p) => <span key={p.id} className="avatar -s" style={p.home ? { borderColor: 'var(--accent)' } : { borderStyle: 'dashed', color: 'var(--fg-faint)' }}>{p.name.charAt(0)}</span>)}
      </span>
      <span className="who-t"><span className="who-n">{txt}</span><span className="who-s">à la maison</span></span>
    </span>
  );
}
export function Who({ person }: { person: Person }) {
  return <span className={cx('who', person.home ? 'is-home' : 'is-away')}><span className="avatar" aria-hidden="true">{person.name.charAt(0)}</span>{person.name} · {person.home ? 'à la maison' : 'absent'}</span>;
}

export const ALERT_ICON: Record<string, IconName | string> = { window_heating: 'window-open', battery: 'battery-low', unavailable: 'wifi-off', smoke: 'smoke', co: 'smoke', offline: 'wifi-off', vigilance: 'alert' };
export function AlertRow({ alert, detail, href, onClick }: { alert: Alert; detail?: string; href?: string; onClick?: () => void }) {
  const tone = alert.level === 'crit' ? 'crit' : alert.level === 'warn' ? 'warn' : alert.kind === 'offline' ? 'info' : undefined;
  const body = (
    <>
      <Pict icon={ALERT_ICON[alert.kind] ?? 'alert'} state={alert.kind === 'unavailable' ? 'unknown' : 'off'} />
      <span className="alert-body"><span className="alert-t">{alert.title}</span>{(detail ?? alert.detail) && <span className="alert-d">{detail ?? alert.detail}</span>}</span>
      {(href || onClick) && <Icon name="chevron" className="alert-go" />}
    </>
  );
  const cls = cx('alert', tone && `is-${tone}`);
  if (href) return <a className={cls} href={href} onClick={onClick}>{body}</a>;
  return <div className={cls}>{body}</div>;
}

export function RoomTile({ name, temperature, pict, right, ambience, image, stale, unavailable, href, onClick }: { name: string; temperature: number | undefined; pict: ReactNode; right?: ReactNode; ambience?: string; image?: string; stale?: boolean; unavailable?: boolean; href?: string; onClick?: () => void }) {
  const cls = cx('room', !!image && 'has-img', stale && 'is-stale', unavailable && 'is-unavailable');
  const inner = (
    <>
      {image ? <Photo src={image} ambience={ambience} /> : <Ambience variant={ambience} />}
      <div className="room-head">{pict}{right}</div>
      <div className="room-body">
        <span className="room-name">{name}</span>
        <span className={cx('val', '-std', temperature == null && 'is-none')}><span className="val-n">{temperature == null ? '—' : temperature.toLocaleString('fr-FR', { maximumFractionDigits: 1 })}</span><span className="val-u">°</span></span>
      </div>
    </>
  );
  if (href) return <a className={cls} href={href} onClick={onClick}>{inner}</a>;
  return <button type="button" className={cls} onClick={onClick}>{inner}</button>;
}

export function BottomNav({ items, active, renderLink }: { items: Array<{ id: string; label: string; icon: IconName | string }>; active: string; renderLink: (item: { id: string; label: string; icon: IconName | string }, cls: string, children: ReactNode) => ReactNode }) {
  return (
    <nav className="nav" aria-label="Navigation">
      {items.map((it) => <span key={it.id} style={{ display: 'contents' }}>{renderLink(it, cx(it.id === active && 'is-active'), <><Icon name={it.icon} />{it.label}</>)}</span>)}
    </nav>
  );
}

export { Dot };

/* ---------- Confirmation sheet and toasts (étape 2) ---------- */

export function Sheet({ icon, tone, title, detail, confirmLabel, danger, onConfirm, onCancel, busy }: { icon: IconName | string; tone?: 'warn' | 'crit' | 'heat' | 'off'; title: string; detail?: string; confirmLabel: string; danger?: boolean; onConfirm: () => void; onCancel: () => void; busy?: boolean }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onCancel(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);
  const node = (
    <div className="scrim" onClick={onCancel} role="presentation">
      <div className="sheet" role="dialog" aria-modal="true" aria-labelledby="sheet-t" onClick={(e) => e.stopPropagation()}>
        <div className="sheet-head"><Pict icon={icon} size="l" state={tone ?? 'off'} /><div><div className="sheet-t" id="sheet-t">{title}</div>{detail && <div className="sheet-d">{detail}</div>}</div></div>
        <div className="sheet-acts">
          <button type="button" className="btn" onClick={onCancel} disabled={busy}>Annuler</button>
          <button type="button" className={cx('btn', danger ? '-danger' : '-primary')} onClick={onConfirm} disabled={busy}>{confirmLabel}</button>
        </div>
      </div>
    </div>
  );
  return typeof document !== 'undefined' ? createPortal(node, document.body) : node;
}

export function Toasts({ items, onDismiss }: { items: Array<{ id: number; title: string; detail?: string; tone?: 'ok' | 'accent' | 'error'; action?: { label: string; run: () => void } }>; onDismiss: (id: number) => void }) {
  if (items.length === 0) return null;
  const node = (
    <div className="toasts" aria-live="polite">
      {items.map((t) => (
        <div key={t.id} className={cx('toast', t.tone === 'error' && 'is-error')} role="status">
          <Dot tone={t.tone === 'error' ? 'crit' : t.tone === 'accent' ? 'accent' : 'ok'} />
          <span><span className="toast-t">{t.title}</span>{t.detail && <> <span className="toast-d">{t.detail}</span></>}</span>
          {t.action ? <button type="button" onClick={() => { t.action!.run(); onDismiss(t.id); }}>{t.action.label}</button> : t.tone === 'error' ? <button type="button" onClick={() => onDismiss(t.id)}>OK</button> : null}
        </div>
      ))}
    </div>
  );
  return typeof document !== 'undefined' ? createPortal(node, document.body) : node;
}


/** A floating panel (same glass sheet as the confirmation) holding arbitrary controls; closes on scrim click or Escape. */
export function Popover({ onClose, children, label }: { onClose: () => void; children: ReactNode; label: string }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  const node = (
    <div className="scrim" onClick={onClose} role="presentation">
      <div className="sheet -panel" role="dialog" aria-modal="true" aria-label={label} onClick={(e) => e.stopPropagation()}>{children}</div>
    </div>
  );
  return typeof document !== 'undefined' ? createPortal(node, document.body) : node;
}
