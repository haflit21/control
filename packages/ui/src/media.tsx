// Room banner (camera or photo), media picker (radio stations, Spotify search
// results) and the camera wall of the Sécurité page.
import { useEffect, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import type { Radio, Track } from '@control/model';
import { Icon, Tag, cx } from './primitives';
import { Ambience } from './composites';
import type { IconName } from './sprite';

/** An <img> that falls back to another source, then to the ambience, when it fails. */
function Img({ src, fallback, ambience, className }: { src?: string; fallback?: string; ambience?: string; className?: string }) {
  const [step, setStep] = useState(0);
  useEffect(() => setStep(0), [src]);
  const url = step === 0 ? src : step === 1 ? fallback : undefined;
  if (!url) return <Ambience variant={ambience} />;
  return <img className={cx('hero-img', className)} src={url} alt="" onError={() => setStep((s) => (s === 0 && fallback ? 1 : 2))} />;
}

/**
 * Room banner across the page. Photo and low when the camera is off (private,
 * unreachable, none); live video at the recording's 16:9 when it is on.
 */
export function RoomBanner({ title, sub, image, video, poster, ambience, live, chips, action, center }: {
  title: string; sub?: string; image?: string; video?: string; poster?: string; ambience?: string; live?: boolean;
  chips?: ReactNode; action?: ReactNode; center?: { icon: IconName | string; text: string };
}) {
  const on = !!video;
  return (
    <section className={cx('rbanner', on && 'is-live')} aria-label={title}>
      {on ? <Img src={video} fallback={poster} ambience={ambience} /> : image ? <Img src={image} ambience={ambience} /> : <Ambience variant={ambience} />}
      <div className="rbanner-top">
        <span className="chip-row">{live && <Tag live>LIVE</Tag>}{chips}</span>
        {action}
      </div>
      {center && <div className="hero-center"><Icon name={center.icon} size="l" />{center.text}</div>}
      <div className="rbanner-body">
        <h2 className="rbanner-t">{title}</h2>
        {sub && <span className="rbanner-s">{sub}</span>}
      </div>
    </section>
  );
}

/** A button on a photo (banner, camera): glass pill with an icon. */
export function PhotoButton({ icon, children, onClick, pending, label }: { icon: IconName | string; children?: ReactNode; onClick: () => void; pending?: boolean; label?: string }) {
  return (
    <button type="button" className={cx('chip -on-photo -btn', pending && 'is-pending')} aria-label={label} onClick={onClick}>
      <Icon name={icon} />{children}
    </button>
  );
}

/** Radio station tile: coloured initials, name, genre; animated bars when it plays. */
export function Station({ radio, playing, pending, onPlay }: { radio: Radio; playing: boolean; pending?: boolean; onPlay: () => void }) {
  return (
    <button type="button" className={cx('station', playing && 'is-on', pending && 'is-pending')} aria-pressed={playing} onClick={onPlay}>
      <span className="station-logo" style={radio.color ? ({ ['--station' as string]: radio.color } as CSSProperties) : undefined} aria-hidden="true">{radio.short}</span>
      <span className="station-body"><span className="station-t">{radio.name}</span>{radio.genre && <span className="station-s">{radio.genre}</span>}</span>
      {playing ? <span className="eq" aria-label="En lecture"><i /><i /><i /><i /></span> : <Icon name="play" />}
    </button>
  );
}

export function minSec(ms: number): string {
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** One music search result: cover, title, artists · album, duration, play. */
export function TrackRow({ track, playing, pending, onPlay }: { track: Track; playing: boolean; pending?: boolean; onPlay: () => void }) {
  const [failed, setFailed] = useState(false);
  return (
    <button type="button" className={cx('track', playing && 'is-on', pending && 'is-pending')} aria-pressed={playing} onClick={onPlay}>
      {track.cover && !failed ? <img className="track-cover" src={track.cover} alt="" loading="lazy" onError={() => setFailed(true)} /> : <span className="track-cover" aria-hidden="true" />}
      <span className="track-body"><span className="track-t">{track.title}</span><span className="track-s">{[track.artists, track.album].filter(Boolean).join(' · ')}</span></span>
      <span className="track-d">{track.durationMs > 0 ? minSec(track.durationMs) : ''}</span>
      <span className="track-play" aria-hidden="true"><Icon name={playing ? 'pause' : 'play'} /></span>
    </button>
  );
}

/** Search input with a magnifier and a clear button. */
export function SearchField({ value, onChange, placeholder, label, busy }: { value: string; onChange: (v: string) => void; placeholder: string; label: string; busy?: boolean }) {
  return (
    <label className={cx('search', busy && 'is-busy')}>
      <Icon name="search" />
      <input type="search" value={value} placeholder={placeholder} aria-label={label} enterKeyHint="search" autoComplete="off" spellCheck={false} onChange={(e) => onChange(e.target.value)} />
      {value && <button type="button" className="search-x" aria-label="Effacer" onClick={() => onChange('')}><Icon name="x" /></button>}
    </label>
  );
}

export type CamItem = { id: string; title: string; sub?: string; image?: string; video?: string; poster?: string; ambience?: string; live: boolean; state?: 'private' | 'offline'; chips?: ReactNode; href?: string };

/**
 * Camera wall: the main camera large (16:9), every camera as a thumbnail; a
 * touch on a thumbnail puts that camera on the big screen.
 */
export function CamWall({ items, active, onSelect, action }: { items: CamItem[]; active: string; onSelect: (id: string) => void; action?: ReactNode }) {
  const main = items.find((c) => c.id === active) ?? items[0];
  if (!main) return null;
  return (
    <div className={cx('camwall', items.length < 2 && '-single')}>
      <div className={cx('camwall-main hero -wide', main.state && `is-${main.state}`)}>
        <Img key={main.id} src={main.video ?? main.image} fallback={main.poster} ambience={main.ambience} />
        <div className="hero-top">{main.live ? <Tag live>LIVE</Tag> : <span />}<div className="chip-row">{main.chips}{action}</div></div>
        {main.state && <div className="hero-center"><Icon name={main.state === 'private' ? 'eye-off' : 'wifi-off'} size="l" />{main.state === 'private' ? 'Mode privé' : 'Injoignable'}</div>}
        <div className="hero-bot">
          <span className="pict"><Icon name="camera" /></span>
          <span className="hero-body"><span className="hero-t">{main.title}</span>{main.sub && <span className="hero-s">{main.sub}</span>}</span>
          {main.href && <a href={main.href} className="camwall-open" aria-label={`Ouvrir ${main.title}`}><Icon name="expand" /></a>}
        </div>
      </div>
      {items.length > 1 && (
        <div className="camwall-thumbs" role="tablist" aria-label="Caméras">
          <span className="camwall-h">Caméras · {items.length}</span>
          {items.map((c) => (
            <button key={c.id} type="button" role="tab" aria-selected={c.id === main.id} className={cx('camthumb', c.id === main.id && 'is-on', c.state && `is-${c.state}`)} onClick={() => onSelect(c.id)}>
              <Img src={c.image} fallback={c.poster} ambience={c.ambience} />
              {c.id === main.id && <span className="camthumb-tag">À l’écran</span>}
              <span className="camthumb-n">{c.live && <i className="dot is-crit" />}{c.title}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
