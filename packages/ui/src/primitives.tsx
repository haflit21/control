import { useEffect, useRef, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { SPRITE, type IconName } from './sprite';
import { temp1, temp0, int, relative, absoluteTime } from './format';

export function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}

/** The icon sprite; render once at the top of the app. */
export function IconSprite() {
  return <svg width="0" height="0" style={{ position: 'absolute' }} aria-hidden="true" dangerouslySetInnerHTML={{ __html: SPRITE }} />;
}

export function Icon({ name, size, className, style, label }: { name: IconName | string; size?: 's' | 'l' | 'xl'; className?: string; style?: CSSProperties; label?: string }) {
  return (
    <svg className={cx('ico', size && `-${size}`, className)} style={style} role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
      <use href={`#i-${name}`} />
    </svg>
  );
}

// ---------- Valeur ----------
export type ValSize = 'giant' | 'hero' | 'std' | 's' | 'meta';
export function Val({ value, unit, size = 'std', decimals = 1, stale, tone, title, className }: {
  value: number | undefined | null; unit?: string; size?: ValSize; decimals?: 0 | 1; stale?: boolean; tone?: 'heat' | 'cool'; title?: string; className?: string;
}) {
  const txt = value == null ? '—' : decimals === 1 ? temp1(value) : decimals === 0 ? temp0(value) : int(value);
  return (
    <span className={cx('val', `-${size}`, value == null && 'is-none', stale && 'is-stale', tone && `is-${tone}`, className)} title={title}>
      <span className="val-n">{txt}</span>
      {unit && <span className="val-u">{unit}</span>}
    </span>
  );
}

export function Stat({ label, meta, children }: { label: string; meta?: string; children: ReactNode }) {
  return (
    <div className="stat">
      <span className="stat-l">{label}</span>
      {children}
      {meta && <span className="stat-m">{meta}</span>}
    </div>
  );
}

// ---------- Pilule ----------
export function Chip({ icon, children, size, onPhoto, tone, className, title }: { icon?: IconName | string; children: ReactNode; size?: 's' | 'l'; onPhoto?: boolean; tone?: string; className?: string; title?: string }) {
  return (
    <span className={cx('chip', size && `-${size}`, onPhoto && '-on-photo', tone && `is-${tone}`, className)} title={title}>
      {icon && <Icon name={icon} />}
      {children}
    </span>
  );
}
export function ChipRow({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx('chip-row', className)}>{children}</div>;
}

// ---------- Paire actuelle → consigne ----------
export function Pair({ current, target, mode, heating, cooling, stale, size = 's', wide, sideLabel, tone }: {
  current: number | undefined; target: number | undefined; mode?: string; heating?: boolean; cooling?: boolean; stale?: boolean; size?: 'std' | 's' | 'meta'; wide?: boolean; sideLabel?: string;
  /** target colour by the house mode: schedule (green), night (purple) */
  tone?: 'schedule' | 'night';
}) {
  const off = mode === 'off';
  const cls = cx('pair', wide && '-wide', heating && 'is-heating', cooling && 'is-cooling', off && 'is-off', tone && !off && `is-${tone}`);
  const targetEl = off ? <span className="pair-target">Arrêt</span> : target == null ? <span className="pair-target">—</span> : (
    <span className="pair-target">
      {wide && heating && <Icon name="flame" className="pair-flame" />}
      <span className="val-n">{temp1(target)}</span><span className="val-u">°</span>
    </span>
  );
  if (wide) {
    return (
      <div className={cls}>
        <Val value={current} unit="°" size="giant" stale={stale} />
        <span className="pair-side"><span className="stat-l">{sideLabel ?? (off ? 'Chauffage' : 'Consigne')}</span>{targetEl}</span>
      </div>
    );
  }
  return (
    <span className={cls}>
      <Val value={current} unit="°" size={size} stale={stale} />
      <Icon name="arrow-right" size="s" className="pair-arrow" />
      {targetEl}
      {heating && !off && <Icon name="flame" className="pair-flame" label="Chauffe" />}
      {cooling && <Icon name="snow" className="pair-flame" label="Refroidit" />}
    </span>
  );
}

// ---------- Point d'état ----------
export type Tone = 'ok' | 'warn' | 'crit' | 'heat' | 'cool' | 'accent' | 'unknown' | 'neutral';
export function Dot({ tone, live, size, label }: { tone?: Tone; live?: boolean; size?: 'm' | 'l'; label?: string }) {
  return <span className={cx('dot', size && `-${size}`, tone && tone !== 'neutral' && `is-${tone}`, live && 'is-live')} role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true} />;
}
export function Status({ tone, live, children, sub, small }: { tone?: Tone; live?: boolean; children: ReactNode; sub?: string; small?: boolean }) {
  return (
    <span className={cx('status', small && '-s')}>
      <Dot tone={tone} live={live} />
      {children}
      {sub && <span className="status-sub">{sub}</span>}
    </span>
  );
}

// ---------- Picto ----------
export type PictState = 'off' | 'lit' | 'on' | 'heat' | 'cool' | 'warn' | 'crit' | 'ok' | 'night' | 'unknown';
export function Pict({ icon, state = 'off', size, count, label, className }: { icon: IconName | string; state?: PictState; size?: 's' | 'l' | 'xl'; count?: number; label?: string; className?: string }) {
  return (
    <span className={cx('pict', size && `-${size}`, state !== 'off' && `is-${state}`, className)} role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
      <Icon name={icon} />
      {count != null && count > 0 && <span className="pict-n">{count}</span>}
    </span>
  );
}
/** A round photo where a Pict would be (device product image). */
export function Thumb({ src, state = 'off', size, className, fallback, tint }: { src: string; state?: 'off' | 'on' | 'unknown'; size?: 's' | 'l' | 'xl'; className?: string; fallback?: ReactNode; tint?: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  if (failed && fallback) return <>{fallback}</>;
  return <img className={cx('thumb', size && `-${size}`, state !== 'off' && `is-${state}`, tint && 'has-tint', className)} style={tint ? ({ ['--tint' as string]: tint } as CSSProperties) : undefined} src={src} alt="" onError={() => setFailed(true)} />;
}
export function Pictv({ icon, state, children }: { icon: IconName | string; state?: 'on' | 'heat' | 'warn' | 'crit'; children?: ReactNode }) {
  return <span className={cx('pictv', state && `is-${state}`)}><Icon name={icon} />{children}</span>;
}

// ---------- Étiquette ----------
export function Tag({ tone, icon, children, live, className }: { tone?: Tone; icon?: IconName | string; children: ReactNode; live?: boolean; className?: string }) {
  return (
    <span className={cx('tag', live && '-live', tone && tone !== 'neutral' && `is-${tone}`, className)}>
      {live && <Dot tone="crit" live />}
      {icon && <Icon name={icon} />}
      {children}
    </span>
  );
}

// ---------- Jauges ----------
export function Gauge({ label, value, max = 100, display, tone, thick, ends, none }: { label: string; value: number | undefined; max?: number; display?: string; tone?: 'heat' | 'cool' | 'warn' | 'crit' | 'ok'; thick?: boolean; ends?: [string, string]; none?: string }) {
  const pct = value == null ? 0 : Math.max(0, Math.min(100, (value / max) * 100));
  return (
    <div className={cx('gauge', tone && `-${tone}`, thick && '-thick', value == null && 'is-none')}>
      <div className="gauge-head"><span>{label}</span><b>{display ?? (value == null ? '—' : `${int(value)} %`)}</b></div>
      <div className="gauge-track" role="meter" aria-valuenow={value ?? undefined} aria-valuemin={0} aria-valuemax={max} aria-label={label}>
        <div className="gauge-fill" style={{ ['--v' as string]: `${pct}%` }} />
      </div>
      {ends && <div className="gauge-ends"><span>{none && value == null ? none : ends[0]}</span><span>{ends[1]}</span></div>}
    </div>
  );
}
export function Dots({ n = 14, on, tone, large, label }: { n?: number; on: number; tone?: 'heat' | 'cool'; large?: boolean; label?: string }) {
  return (
    <span className={cx('dots', tone && `-${tone}`, large && '-l')} role="meter" aria-valuenow={on} aria-valuemin={0} aria-valuemax={n} aria-label={label}>
      {Array.from({ length: n }, (_, i) => <i key={i} className={i < on ? 'is-on' : undefined} />)}
    </span>
  );
}
export function Arc({ value, label, tone, children, ends = ['0', '100'] }: { value: number | undefined; label: string; tone?: 'heat'; children: ReactNode; ends?: [string, string] }) {
  const v = Math.max(0, Math.min(100, value ?? 0));
  const N = 33, R = 44, cx0 = 50, cy = 52;
  const ticks = Array.from({ length: N }, (_, i) => {
    const t = i / (N - 1), ang = ((210 - t * 240) * Math.PI) / 180, len = i % 4 === 0 ? 9 : 6;
    return <line key={i} x1={cx0 + Math.cos(ang) * R} y1={cy - Math.sin(ang) * R} x2={cx0 + Math.cos(ang) * (R - len)} y2={cy - Math.sin(ang) * (R - len)} className={cx('tick', value != null && v > 0 && t <= v / 100 && 'is-on')} />;
  });
  return (
    <div className={cx('arc', tone && `-${tone}`)} role="meter" aria-valuenow={value ?? undefined} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
      <svg viewBox="0 0 100 76" aria-hidden="true">{ticks}</svg>
      <div className="arc-c"><span className="arc-l">{label}</span>{children}</div>
      <div className="arc-ends"><span>{ends[0]}</span><span>{ends[1]}</span></div>
    </div>
  );
}

// ---------- Bascule et bouton marche (display only in v1) ----------
/** Toggle: a real button when onChange is given (étape 2), a passive indicator otherwise. */
export function Toggle({ on, busy, pending, small, label, onChange, disabled }: { on: boolean; busy?: boolean; pending?: boolean; small?: boolean; label: string; onChange?: (on: boolean) => void; disabled?: boolean }) {
  const cls = cx('toggle', small && '-s', on && 'is-on', busy && 'is-busy', pending && 'is-pending');
  if (onChange) {
    return <button type="button" className={cls} role="switch" aria-checked={on} aria-label={label} disabled={disabled} onClick={(e) => { e.preventDefault(); e.stopPropagation(); onChange(!on); }} />;
  }
  return <span className={cls} role="img" aria-label={label} />;
}
/** Power button: same contract as Toggle. */
export function Pwr({ on, icon = 'power', label, onClick, pending, disabled }: { on: boolean; icon?: IconName | string; label: string; onClick?: () => void; pending?: boolean; disabled?: boolean }) {
  const cls = cx('pwr', on && 'is-on', pending && 'is-pending');
  if (onClick) {
    return <button type="button" className={cls} aria-pressed={on} aria-label={label} disabled={disabled} onClick={(e) => { e.preventDefault(); e.stopPropagation(); onClick(); }}><Icon name={icon} /></button>;
  }
  return <span className={cls} role="img" aria-label={label}><Icon name={icon} /></span>;
}

/** − value + ; the value is committed `delay` ms after the last press (debounced). */
export function Stepper({ value, min, max, step = 0.5, unit = '°', heat, large, label, onCommit, delay = 600, pending, format, fallback }: { value: number | undefined; min: number; max: number; step?: number; unit?: string; heat?: boolean; large?: boolean; label: string; onCommit: (v: number) => void; delay?: number; pending?: boolean; format?: (v: number) => string; fallback?: number }) {
  const [draft, setDraft] = useState<number | undefined>(undefined);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => { setDraft(undefined); }, [value]);
  const v = draft ?? value;
  const bump = (dir: 1 | -1) => {
    const base = v ?? fallback;
    if (base == null) return;
    const next = v == null ? base : Math.round(Math.min(max, Math.max(min, base + dir * step)) * 100) / 100;
    setDraft(next);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => { onCommit(next); }, delay);
  };
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const text = v == null ? '—' : format ? format(v) : v.toLocaleString('fr-FR', { minimumFractionDigits: 0, maximumFractionDigits: 1 });
  return (
    <div className={cx('stepper', heat && '-heat', large && '-l', (pending || draft != null) && 'is-pending')} role="group" aria-label={label}>
      <button type="button" aria-label="Moins" disabled={(v == null && fallback == null) || (v != null && v <= min)} onClick={() => bump(-1)}><Icon name="minus" /></button>
      <span className="stepper-v" aria-live="polite">{text}{v != null && <span className="val-u">{unit}</span>}</span>
      <button type="button" aria-label="Plus" disabled={(v == null && fallback == null) || (v != null && v >= max)} onClick={() => bump(1)}><Icon name="plus" /></button>
    </div>
  );
}

/** Range slider: commits on release (and at most every `throttle` ms while dragging when `live`). */
export function Slider({ value, min = 0, max = 100, step = 1, label, ends, display, cool, onCommit, live, throttle = 400, pending, disabled, className, style, onDraft }: { value: number | undefined; min?: number; max?: number; step?: number; label: string; ends?: [string, string]; display?: (v: number) => string; cool?: boolean; onCommit: (v: number) => void; live?: boolean; throttle?: number; pending?: boolean; disabled?: boolean; className?: string; style?: CSSProperties; onDraft?: (v: number) => void }) {
  const [draft, setDraft] = useState<number | undefined>(undefined);
  const last = useRef(0);
  useEffect(() => { setDraft(undefined); }, [value]);
  const v = draft ?? value ?? min;
  const pct = ((v - min) / (max - min)) * 100;
  const fmt = display ?? ((x: number) => `${Math.round(x)} %`);
  const commit = (x: number) => { setDraft(x); onCommit(x); };
  return (
    <div className={cx('slider', cool && '-cool', (pending || draft != null) && 'is-pending', className)} style={style}>
      <div className="slider-head"><span>{label}</span><b>{value == null && draft == null ? '—' : fmt(v)}</b></div>
      <input type="range" min={min} max={max} step={step} value={v} disabled={disabled} aria-label={label} style={{ ['--v' as string]: `${pct}%` } as CSSProperties}
        onChange={(e) => { const x = Number(e.target.value); setDraft(x); onDraft?.(x); if (live && Date.now() - last.current > throttle) { last.current = Date.now(); onCommit(x); } }}
        onPointerUp={(e) => commit(Number((e.target as HTMLInputElement).value))}
        onKeyUp={(e) => { if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'PageUp', 'PageDown'].includes(e.key)) commit(Number((e.target as HTMLInputElement).value)); }} />
      {ends && <div className="slider-ends"><span>{ends[0]}</span><span>{ends[1]}</span></div>}
    </div>
  );
}

export function Btn({ children, kind, onClick, disabled, type = 'button' }: { children: ReactNode; kind?: 'primary' | 'danger' | 'ghost'; onClick?: () => void; disabled?: boolean; type?: 'button' | 'submit' }) {
  return <button type={type} className={cx('btn', kind && `-${kind}`)} onClick={onClick} disabled={disabled}>{children}</button>;
}

// ---------- Temps relatif ----------
export function TimeAgo({ iso, now, prefix, old, live, icon }: { iso: string | undefined; now: Date; prefix?: 'il y a' | 'depuis' | 'dans' | ''; old?: boolean; live?: boolean; icon?: boolean }) {
  return (
    <time className={cx('time', old && 'is-old', live && 'is-live')} dateTime={iso} title={absoluteTime(iso)}>
      {icon && <Icon name="clock" size="s" />}
      {relative(iso, now, prefix)}
    </time>
  );
}

// ---------- Compteur-phrase ----------
export function Phrase({ icon, count, one, many, zero, tone, large }: { icon: IconName | string; count: number; one: string; many: string; zero: string; tone?: 'heat' | 'on' | 'warn' | 'good'; large?: boolean }) {
  const isZero = count === 0;
  return (
    <span className={cx('phrase', large && '-l', isZero ? 'is-zero' : tone && `is-${tone}`)}>
      <Icon name={isZero && tone === 'good' ? 'check' : icon} />
      {isZero ? zero : <><b>{int(count)}</b>&nbsp;{count === 1 ? one : many}</>}
    </span>
  );
}

// ---------- Surfaces ----------
export function Card({ children, tight, solid, hero, stale, unavailable, crit, className, as: As = 'div', onClick, href, style }: {
  children: ReactNode; tight?: boolean; solid?: boolean; hero?: boolean; stale?: boolean; unavailable?: boolean; crit?: boolean; className?: string; as?: 'div' | 'a' | 'button'; onClick?: () => void; href?: string; style?: CSSProperties;
}) {
  const cls = cx('card', tight && '-tight', solid && '-solid', hero && '-hero', stale && 'is-stale', unavailable && 'is-unavailable', crit && 'is-crit', className);
  if (As === 'a') return <a className={cls} href={href} onClick={onClick} style={style}>{children}</a>;
  if (As === 'button') return <button type="button" className={cls} onClick={onClick} style={style}>{children}</button>;
  return <div className={cls} style={style}>{children}</div>;
}
export function CardHead({ pict, title, sub, right, small }: { pict: ReactNode; title: string; sub?: ReactNode; right?: ReactNode; small?: boolean }) {
  return (
    <div className="card-head">
      {pict}
      <div style={{ display: 'grid', minWidth: 0 }}>
        <span className={cx('card-title', small && '-s')}>{title}</span>
        {sub && <span className="card-sub">{sub}</span>}
      </div>
      {right && <span style={{ marginLeft: 'auto', flex: 'none' }}>{right}</span>}
    </div>
  );
}
export function Section({ title, count, right, children }: { title: string; count?: string; right?: ReactNode; children?: ReactNode }) {
  return (
    <div>
      <div className="sec"><h2>{title}{count && <small>{count}</small>}</h2>{right}</div>
      {children}
    </div>
  );
}
export function Row({ pict, name, sub, right, href, onClick }: { pict: ReactNode; name: string; sub?: ReactNode; right?: ReactNode; href?: string; onClick?: () => void }) {
  const body = <>{pict}<span><span className="row-name">{name}</span>{sub && <span className="row-sub">{sub}</span>}</span>{right}</>;
  if (href) return <a className="row" href={href} onClick={onClick}>{body}</a>;
  return <div className="row">{body}</div>;
}
