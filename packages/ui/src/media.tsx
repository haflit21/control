// Room banner (camera or photo), speaker player, media picker (radio
// stations, music search results, albums) and the camera wall of the Sécurité page.
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import type { Album, Radio, Track } from '@control/model';
import { Icon, Slider, Tag, cx } from './primitives';
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

/** A radio's logo, or its initials on its colour. */
export function StationLogo({ radio, className }: { radio: Radio; className?: string }) {
  const [failed, setFailed] = useState(false);
  if (radio.logo && !failed) return <img className={cx('station-logo -img', className)} src={radio.logo} alt="" onError={() => setFailed(true)} />;
  return <span className={cx('station-logo', className)} style={radio.color ? ({ ['--station' as string]: radio.color } as CSSProperties) : undefined} aria-hidden="true">{radio.short}</span>;
}

/** Radio station tile: logo, name, genre; animated bars when it plays. */
export function Station({ radio, playing, pending, onPlay }: { radio: Radio; playing: boolean; pending?: boolean; onPlay: () => void }) {
  return (
    <button type="button" className={cx('station', playing && 'is-on', pending && 'is-pending')} aria-pressed={playing} onClick={onPlay}>
      <StationLogo radio={radio} />
      <span className="station-body"><span className="station-t">{radio.name}</span>{radio.genre && <span className="station-s">{radio.genre}</span>}</span>
      {playing ? <span className="eq" aria-label="En lecture"><i /><i /><i /><i /></span> : <Icon name="play" />}
    </button>
  );
}

export function minSec(ms: number): string {
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** Star toggle: in the favourites or not. */
export function FavButton({ on, onToggle, label }: { on: boolean; onToggle: () => void; label: string }) {
  return (
    <button type="button" className={cx('fav', on && 'is-on')} aria-pressed={on} aria-label={on ? `Retirer ${label} des favoris` : `Ajouter ${label} aux favoris`}
      onClick={(e) => { e.stopPropagation(); onToggle(); }}>
      <Icon name="star" />
    </button>
  );
}

/** One music search result: cover, title, artists · album, duration, favourite, play. */
export function TrackRow({ track, playing, pending, onPlay, fav, onFav }: { track: Track; playing: boolean; pending?: boolean; onPlay: () => void; fav?: boolean; onFav?: () => void }) {
  const [failed, setFailed] = useState(false);
  return (
    <div className={cx('track', playing && 'is-on', pending && 'is-pending')}>
      <button type="button" className="track-hit" aria-pressed={playing} aria-label={`${playing ? 'Mettre en pause' : 'Écouter'} ${track.title}`} onClick={onPlay}>
        {track.cover && !failed ? <img className="track-cover" src={track.cover} alt="" loading="lazy" onError={() => setFailed(true)} /> : <span className="track-cover" aria-hidden="true" />}
        <span className="track-body"><span className="track-t">{track.title}</span><span className="track-s">{[track.artists, track.album].filter(Boolean).join(' · ')}</span></span>
        <span className="track-d">{track.durationMs > 0 ? minSec(track.durationMs) : ''}</span>
      </button>
      {onFav && <FavButton on={!!fav} onToggle={onFav} label={track.title} />}
      <button type="button" className="track-play" tabIndex={-1} aria-hidden="true" onClick={onPlay}><Icon name={playing ? 'pause' : 'play'} /></button>
    </div>
  );
}

/** Album tile: cover, title, artists · year; a touch plays the album, the star keeps it. */
export function AlbumTile({ album, playing, pending, onPlay, fav, onFav }: { album: Album; playing: boolean; pending?: boolean; onPlay: () => void; fav?: boolean; onFav?: () => void }) {
  const [failed, setFailed] = useState(false);
  return (
    <div className={cx('album', playing && 'is-on', pending && 'is-pending')}>
      <button type="button" className="album-hit" aria-pressed={playing} aria-label={`Écouter l’album ${album.title}`} onClick={onPlay}>
        <span className="album-cover">
          {album.cover && !failed ? <img src={album.cover} alt="" loading="lazy" onError={() => setFailed(true)} /> : <Icon name="disc" />}
          <span className="album-play" aria-hidden="true">{playing ? <span className="eq"><i /><i /><i /><i /></span> : <Icon name="play" />}</span>
        </span>
        <span className="album-t">{album.title}</span>
        <span className="album-s">{[album.artists, album.year].filter(Boolean).join(' · ')}</span>
      </button>
      {onFav && <FavButton on={!!fav} onToggle={onFav} label={album.title} />}
    </div>
  );
}

function clock(sec: number): string {
  return minSec(Math.max(0, sec) * 1000);
}

/**
 * What a speaker plays and its controls: artwork (or the radio's logo),
 * title, artist; the progress bar to seek in a track, or « En direct » for a
 * radio; previous / play-pause / next; the volume.
 */
export function MediaPlayer({ title, sub, app, cover, radio, playing, off, pending, elapsed, elapsedAt, duration, canPrev, canNext, canSeek, volume, onToggle, onPrev, onNext, onSeek, onVolume, onPowerOff }: {
  title: string; sub?: string; app?: string; cover?: string; radio?: Radio; playing: boolean; off?: boolean; pending?: boolean;
  elapsed?: number; elapsedAt?: string; duration?: number; canPrev?: boolean; canNext?: boolean; canSeek?: boolean; volume?: number;
  onToggle: () => void; onPrev?: () => void; onNext?: () => void; onSeek?: (sec: number) => void; onVolume?: (pct: number) => void; onPowerOff?: () => void;
}) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [cover]);
  const [now, setNow] = useState(() => Date.now());
  const ticking = playing && duration != null && elapsed != null;
  useEffect(() => {
    if (!ticking) return;
    setNow(Date.now());
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [ticking, elapsedAt]);
  let pos = elapsed;
  if (pos != null && playing && elapsedAt) {
    const at = Date.parse(elapsedAt);
    if (!Number.isNaN(at)) pos += Math.max(0, (now - at) / 1000);
  }
  if (pos != null && duration != null) pos = Math.min(pos, duration);
  // While the finger is on the bar: the value under it. After a seek: the asked
  // position, until the speaker reports one near it (it keeps sending the old
  // one for a moment), at most 8 s.
  const [draft, setDraft] = useState<number | undefined>(undefined);
  const hold = useRef<{ target: number; at: number; playing: boolean } | null>(null);
  if (hold.current) {
    const h = hold.current;
    const expected = h.target + (h.playing ? (now - h.at) / 1000 : 0);
    if ((pos != null && Math.abs(pos - expected) < 3) || Date.now() - h.at > 8000) hold.current = null;
    else pos = duration != null ? Math.min(expected, duration) : expected;
  }
  const shown = draft ?? pos ?? 0;
  const commit = (v: number) => {
    setDraft(undefined);
    hold.current = { target: v, at: Date.now(), playing };
    setNow(Date.now());
    onSeek?.(v);
  };
  const live = !!radio;
  return (
    <div className={cx('mp', !playing && 'is-idle', off && 'is-off', pending && 'is-pending')}>
      <div className="mp-head">
        {radio ? <StationLogo radio={radio} className="mp-cover" />
          : cover && !failed ? <img className="mp-cover" src={cover} alt="" onError={() => setFailed(true)} />
            : <span className="mp-cover -empty" aria-hidden="true"><Icon name="music" /></span>}
        <span className="mp-body">
          {app && <span className="mp-app">{app}</span>}
          <span className="mp-t">{title}</span>
          {sub && <span className="mp-s">{sub}</span>}
        </span>
        {onPowerOff && <button type="button" className="mp-btn -power" aria-label="Éteindre l’enceinte" disabled={off} onClick={onPowerOff}><Icon name="power" /></button>}
      </div>
      {live ? (
        <div className="mp-prog -live"><span className="mp-live"><i className="dot" />En direct</span></div>
      ) : duration != null && duration > 0 ? (
        <div className="mp-prog">
          <span>{clock(shown)}</span>
          <input type="range" className="mp-seek" min={0} max={Math.round(duration)} step={1} value={Math.round(shown)} disabled={!canSeek || !onSeek}
            aria-label="Position dans le titre" aria-valuetext={`${clock(shown)} sur ${clock(duration)}`}
            style={{ ['--v' as string]: `${(shown / duration) * 100}%` } as CSSProperties}
            onChange={(e) => setDraft(Number(e.target.value))}
            onPointerUp={(e) => commit(Number((e.target as HTMLInputElement).value))}
            onPointerCancel={() => setDraft(undefined)}
            onKeyUp={(e) => { if (['ArrowLeft', 'ArrowRight', 'Home', 'End', 'PageUp', 'PageDown'].includes(e.key)) commit(Number((e.target as HTMLInputElement).value)); }} />
          <span>{clock(duration)}</span>
        </div>
      ) : null}
      <div className="mp-ctrl">
        {!live && <button type="button" className="mp-btn" aria-label="Titre précédent" disabled={!canPrev || !onPrev} onClick={onPrev}><Icon name="skip-back" /></button>}
        <button type="button" className={cx('mp-btn -play', playing && 'is-on')} aria-pressed={playing} aria-label={playing ? 'Mettre en pause' : 'Lecture'} onClick={onToggle}><Icon name={playing ? 'pause' : 'play'} /></button>
        {!live && <button type="button" className="mp-btn" aria-label="Titre suivant" disabled={!canNext || !onNext} onClick={onNext}><Icon name="skip-forward" /></button>}
      </div>
      {volume != null && onVolume && (
        <div className="mp-vol">
          <Icon name="volume-low" />
          <Slider label="Volume" value={volume} cool pending={pending} onCommit={onVolume} />
          <Icon name="volume" />
        </div>
      )}
    </div>
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
