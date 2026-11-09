import type { ReactNode } from 'react';
import type { Air, ForecastDay, ForecastHour, PollenKey, PollutantKey, Weather } from '@control/model';
import { Icon, Val, cx } from './primitives';
import { conditionLabel, temp0, int, dayShort, hourShort, cardinal, uvLevel, relative, absoluteTime } from './format';

// ---- glyph: same structure as widgets.js weatherGlyph ----
const CLOUD = 'M28 80a17 17 0 0 1-3-33.7A24 24 0 0 1 70 50a15 15 0 0 1-1 30z';
const CLOUD_BACK = 'M44 58a13 13 0 0 1-1.6-25.9A18 18 0 0 1 78 36a11 11 0 0 1 1 22z';
type Fx = Array<['path', { d: string; className: string }] | ['circle', { cx: number; cy: number; r: number; className: string }]>;
const FX: Record<string, Fx> = {
  rain: [['path', { d: 'M13 4l-4 13M30 4l-4 13M47 4l-4 13', className: 'g-rain' }]],
  pouring: [['path', { d: 'M8 2l-5 18M20 2l-5 18M32 2l-5 18M44 2l-5 18M56 2l-5 18', className: 'g-rain -heavy' }]],
  snow: [['path', { d: 'M12 3v14M6 6.5l12 7M18 6.5l-12 7M30 3v14M24 6.5l12 7M36 6.5l-12 7M48 3v14M42 6.5l12 7M54 6.5l-12 7', className: 'g-snow' }]],
  sleet: [['path', { d: 'M12 4l-4 13M48 4l-4 13', className: 'g-rain' }], ['path', { d: 'M30 3v14M24 6.5l12 7M36 6.5l-12 7', className: 'g-snow' }]],
  hail: [['circle', { cx: 12, cy: 10, r: 4, className: 'g-hail' }], ['circle', { cx: 30, cy: 14, r: 4, className: 'g-hail' }], ['circle', { cx: 48, cy: 8, r: 4, className: 'g-hail' }]],
  bolt: [['path', { d: 'M34 0l-12 16h9l-7 16 16-20h-9l7-12z', className: 'g-bolt' }]],
  boltrain: [['path', { d: 'M12 4l-4 13M52 4l-4 13', className: 'g-rain' }], ['path', { d: 'M34 0l-12 16h9l-7 16 16-20h-9l7-12z', className: 'g-bolt' }]],
  fog: [['path', { d: 'M4 6h52M10 16h40M18 26h26', className: 'g-fog' }]],
  wind: [['path', { d: 'M4 8h36a7 7 0 1 0-7-7M4 20h50a7 7 0 1 1-7 7M4 30h24', className: 'g-wind' }]],
  bang: [['path', { d: 'M30 2L14 30h32z', className: 'g-bang' }], ['path', { d: 'M30 12v9M30 25v.5', className: 'g-bang-i' }]],
};
type Spec = { disc?: 'sun' | 'moon' | 'auto'; small?: boolean; cloud?: boolean; back?: boolean; fx?: string; low?: boolean; dark?: boolean; wind?: boolean };
const COND: Record<string, Spec> = {
  sunny: { disc: 'sun' }, 'clear-night': { disc: 'moon' }, partlycloudy: { disc: 'auto', small: true, cloud: true }, cloudy: { cloud: true, back: true },
  fog: { cloud: true, fx: 'fog', low: true }, rainy: { cloud: true, fx: 'rain' }, pouring: { cloud: true, fx: 'pouring', dark: true },
  lightning: { cloud: true, fx: 'bolt', dark: true }, 'lightning-rainy': { cloud: true, fx: 'boltrain', dark: true }, snowy: { cloud: true, fx: 'snow' },
  'snowy-rainy': { cloud: true, fx: 'sleet' }, hail: { cloud: true, fx: 'hail', dark: true }, windy: { wind: true }, 'windy-variant': { cloud: true, fx: 'wind' },
  exceptional: { cloud: true, fx: 'bang', dark: true },
};

function FxSvg({ fx, solo }: { fx: Fx; solo?: boolean }) {
  return (
    <svg viewBox="0 0 60 32" className={cx('g-fx', solo && '-solo')}>
      {fx.map((f, i) => (f[0] === 'path' ? <path key={i} d={f[1].d} className={f[1].className} /> : <circle key={i} cx={f[1].cx} cy={f[1].cy} r={f[1].r} className={f[1].className} />))}
    </svg>
  );
}

export function WeatherGlyph({ cond, night, size, label, hidden }: { cond: string | undefined; night?: boolean; size?: 'xs' | 's' | 'm' | 'xl'; label?: string; hidden?: boolean }) {
  const spec = COND[cond ?? ''] ?? COND.cloudy;
  const moon = spec.disc === 'moon' || (!!night && !!spec.disc);
  return (
    <div className={cx('wx-glyph', size && `-${size}`, night && '-night', spec.dark && '-dark')} role={hidden ? undefined : 'img'} aria-label={hidden ? undefined : label ?? conditionLabel(cond, night)} aria-hidden={hidden ? true : undefined}>
      {spec.disc && <i className={cx('g-disc', moon && '-moon', spec.small && '-small')} />}
      {spec.cloud && (
        <svg viewBox="0 0 100 100" className={cx('g-cloud', spec.small && '-with-disc', spec.low && '-low')}>
          {spec.back && <path d={CLOUD_BACK} className="g-cloud-back" />}
          <path d={CLOUD} />
        </svg>
      )}
      {spec.wind && <FxSvg fx={FX.wind} solo />}
      {spec.fx && <FxSvg fx={FX[spec.fx]} />}
    </div>
  );
}

// ---- hero / compact ----
/** Compact weather pill for a page header: glyph, temperature, condition. */
export function WxPill({ w, now, href, onClick }: { w: Weather; now: Date; href?: string; onClick?: () => void }) {
  const old = now.getTime() - new Date(w.updatedAt).getTime() > 30 * 60 * 1000;
  const inner = (
    <>
      {w.available ? <WeatherGlyph cond={w.condition} night={w.night} size="xs" hidden /> : <span className="pict -s is-unknown"><Icon name="wifi-off" /></span>}
      <span className="who-t">
        <span className="who-n">{w.available && w.temperature != null ? <>{temp0(w.temperature)}<span className="val-u">°</span></> : '—'}</span>
        <span className={cx('who-s', old && 'is-old')}>{w.available ? conditionLabel(w.condition, w.night) : 'Météo indisponible'}</span>
      </span>
    </>
  );
  const cls = 'who-pill wx-pill';
  if (href) return <a className={cls} href={href} onClick={onClick} aria-label={`Météo : ${w.available ? conditionLabel(w.condition, w.night) : 'indisponible'}`}>{inner}</a>;
  return <span className={cls}>{inner}</span>;
}

export function WxHero({ w, now, compact }: { w: Weather; now: Date; compact?: boolean }) {
  const old = now.getTime() - new Date(w.updatedAt).getTime() > 30 * 60 * 1000;
  if (!w.available) {
    return (
      <div className="wx -compact">
        <span className="pict -l is-unknown"><Icon name="wifi-off" /></span>
        <div className="wx-main"><Val value={undefined} unit="°" size="std" /><span className="wx-cond" style={{ color: 'var(--fg-muted)' }}>Météo indisponible</span><span className="wx-updated">dernière mesure {relative(w.updatedAt, now, '')}</span></div>
      </div>
    );
  }
  if (compact) {
    return (
      <div className="wx -compact">
        <WeatherGlyph cond={w.condition} night={w.night} size="s" hidden />
        <div className="wx-main">
          <Val value={w.temperature} unit="°" size="std" decimals={0} stale={old} />
          <span className="wx-cond">{conditionLabel(w.condition, w.night)}</span>
          <span className={cx('wx-updated', old && 'is-old')}>{relative(w.updatedAt, now)}</span>
        </div>
      </div>
    );
  }
  return (
    <div className="wx -hero">
      <div className="wx-main">
        <Val value={w.temperature} unit="°" size="hero" decimals={0} stale={old} />
        <span className="wx-cond">{conditionLabel(w.condition, w.night)}</span>
        {(w.feels != null || w.windSpeed != null) && <span className="wx-feel">{w.feels != null && `Ressenti ${temp0(w.feels)}°`}{w.feels != null && w.windSpeed != null && ' · '}{w.windSpeed != null && `vent ${int(w.windSpeed)} km/h`}</span>}
        <span className="wx-range">
          {w.min != null && <span><Icon name="arrow-down" /> {temp0(w.min)}°</span>}
          {w.max != null && <span><Icon name="arrow-up" /> {temp0(w.max)}°</span>}
          {w.place && <span>{w.place}</span>}
        </span>
        <span className={cx('wx-updated', old && 'is-old')}>{old && <Icon name="clock" size="s" />} {relative(w.updatedAt, now)}</span>
      </div>
      <WeatherGlyph cond={w.condition} night={w.night} hidden />
    </div>
  );
}

export function WxHours({ hours, now, full, night }: { hours: ForecastHour[]; now: Date; full?: boolean; night?: boolean }) {
  const isNight = (iso: string) => { const h = new Date(iso).getHours(); return h < 7 || h >= 19 || (night && h >= 17); };
  return (
    <div className={cx('wx-hours', full && '-full')} role="region" aria-label="Prévision heure par heure" tabIndex={0}>
      {hours.map((h, i) => (
        <div key={h.time} className={cx('wx-h', i === 0 && 'is-now')}>
          <span>{hourShort(h.time, now)}</span>
          <WeatherGlyph cond={h.condition} night={isNight(h.time)} size="xs" hidden />
          <b>{temp0(h.temperature)}°</b>
          {full && <span className={cx('wx-pop', !h.precipProb && 'is-zero')}><Icon name="drop" />{int(h.precipProb ?? 0)}&#8239;%</span>}
        </div>
      ))}
    </div>
  );
}

export function WxDays({ days, now, full }: { days: ForecastDay[]; now: Date; full?: boolean }) {
  const lo = Math.min(...days.map((d) => d.min ?? 99)), hi = Math.max(...days.map((d) => d.max ?? -99));
  const span = Math.max(1, hi - lo);
  return (
    <div className={cx('wx-days', full && '-full')}>
      {days.map((d, i) => {
        const a = d.min == null ? 0 : ((d.min - lo) / span) * 100, b = d.max == null ? 0 : ((hi - d.max) / span) * 100;
        return (
          <div key={d.date} className={cx('wx-d', i === 0 && 'is-today')}>
            <span>{dayShort(d.date, now)}</span>
            <WeatherGlyph cond={d.condition} size="xs" hidden />
            {full && <span className={cx('wx-pop', !d.precipProb && 'is-zero')}><Icon name="drop" />{int(d.precipProb ?? 0)}&#8239;%</span>}
            <span className="wx-min">{temp0(d.min)}°</span>
            <span className="wx-bar" style={{ ['--a' as string]: `${a}%`, ['--b' as string]: `${b}%` }} />
            <span className="wx-max">{temp0(d.max)}°</span>
          </div>
        );
      })}
    </div>
  );
}

export function WxStat({ icon, label, value, unit, sub, decimals = 0, none }: { icon: string; label: string; value: number | undefined; unit?: string; sub?: string; decimals?: 0 | 1; none?: string }) {
  return (
    <div className={cx('wx-stat', value == null && 'is-none')}>
      <div className="wx-stat-h"><Icon name={icon} />{label}</div>
      <Val value={value} unit={unit} size="s" decimals={decimals} />
      {(sub || (value == null && none)) && <span className="wx-stat-s">{value == null ? none ?? sub : sub}</span>}
    </div>
  );
}

export function Compass({ speed, bearing, gust, large }: { speed: number | undefined; bearing: number | undefined; gust?: number; large?: boolean }) {
  const calm = speed == null || speed < 1;
  const force = speed == null ? 0 : speed < 6 ? 1 : speed < 12 ? 2 : speed < 20 ? 3 : speed < 29 ? 4 : speed < 39 ? 5 : speed < 50 ? 6 : speed < 62 ? 7 : 8;
  const gustForce = gust == null ? force : gust < 20 ? force : gust < 39 ? Math.max(force, 5) : gust < 62 ? Math.max(force, 7) : 8;
  const forceLabel = ['—', 'Très faible', 'Léger', 'Petite brise', 'Modéré', 'Assez fort', 'Fort', 'Très fort', 'Tempête'][force];
  return (
    <div className="wind">
      <div className={cx('compass', large && '-l')} style={{ ['--deg' as string]: `${bearing ?? 0}deg` }} role="img" aria-label={calm ? 'Vent calme' : `Vent de ${cardinal(bearing, true)}, ${int(speed)} km/h`}>
        <b>N</b><b>E</b><b>S</b><b>O</b>
        {!calm && <div className="compass-arrow" />}
        <div className="compass-c">{calm ? <><Val value={undefined} size="s" /><small>calme</small></> : <><Val value={speed} size="s" decimals={0} /><small>km/h</small></>}</div>
      </div>
      <div className="wind-facts">
        <div className="wind-fact"><span>Direction</span><b style={calm ? { color: 'var(--fg-faint)' } : undefined}>{calm ? '—' : `${cardinal(bearing, true)} · ${int(bearing)}°`}</b></div>
        <div className="wind-fact"><span>Rafales</span><b style={gust != null && speed != null && gust > speed * 1.5 ? { color: 'var(--warn-fg)' } : gust == null ? { color: 'var(--fg-faint)' } : undefined}>{gust == null ? '—' : `${int(gust)} km/h`}</b></div>
        <div className="wind-fact"><span>Force</span><b>{forceLabel}</b></div>
        <div className="wind-scale" aria-hidden="true">{Array.from({ length: 8 }, (_, i) => <i key={i} className={cx(i < gustForce && 'is-on', i >= force && i < gustForce && '-gust')} />)}</div>
      </div>
    </div>
  );
}

export function UV({ uv }: { uv: number | undefined }) {
  const lvl = uvLevel(uv);
  return (
    <div className={cx('uv', uv == null && 'is-none')}>
      <div className="uv-head"><span>Indice UV</span><b style={uv == null ? { color: 'var(--fg-faint)' } : undefined}>{uv == null ? '—' : <>{int(uv)} · <span className={`uv-lvl lv${lvl?.lv}`}>{lvl?.label}</span></>}</b></div>
      <div className="uv-bar" style={{ ['--v' as string]: uv ?? 0 }} role="meter" aria-valuenow={uv} aria-valuemin={0} aria-valuemax={11} aria-label="Indice UV" />
      <div className="uv-ends"><span>{uv == null ? 'Pas la nuit' : '0'}</span><span>{uv == null ? '' : '11+'}</span></div>
    </div>
  );
}

export function Pressure({ hpa, trend }: { hpa: number | undefined; trend?: 'up' | 'down' | 'flat' }) {
  return (
    <div className="press">
      <div className="press-head">
        <Val value={hpa} unit="hPa" size="s" decimals={0} />
        {trend && <span className={cx('press-trend', trend === 'up' && '-up', trend === 'down' && '-down')}><Icon name={trend === 'up' ? 'arrow-up' : trend === 'down' ? 'arrow-down' : 'minus'} />{trend === 'up' ? 'en hausse' : trend === 'down' ? 'en baisse' : 'stable'}</span>}
      </div>
      <div className="press-bar" style={{ ['--v' as string]: hpa ?? 1013 }} role="meter" aria-valuenow={hpa} aria-valuemin={980} aria-valuemax={1040} aria-label="Pression" />
      <div className="press-ends"><span>980</span><span>Dépression · anticyclone</span><span>1&#8239;040</span></div>
    </div>
  );
}

export function SunArc({ sunrise, sunset, now }: { sunrise?: string; sunset?: string; now: Date }) {
  // position of the sun along today's arc; at night the moon runs between sunset and the next sunrise
  const rise = sunrise ? new Date(sunrise) : undefined, set = sunset ? new Date(sunset) : undefined;
  if (!rise || !set) return null;
  // HA gives *next* rising / setting. Day if the next setting comes before the next rising.
  const isDay = set.getTime() < rise.getTime();
  let p: number, left: ReactNode, right: ReactNode, caption: string;
  if (isDay) {
    const todayRise = new Date(rise.getTime() - 24 * 3600 * 1000);
    p = (now.getTime() - todayRise.getTime()) / (set.getTime() - todayRise.getTime());
    left = <span>Lever<b>{absoluteTime(todayRise.toISOString())}</b></span>;
    right = <span>Coucher<b>{absoluteTime(set.toISOString())}</b></span>;
    const dur = (set.getTime() - todayRise.getTime()) / 60000;
    caption = `${Math.floor(dur / 60)} h ${String(Math.round(dur % 60)).padStart(2, '0')} de jour`;
  } else {
    const lastSet = new Date(set.getTime() - 24 * 3600 * 1000);
    p = (now.getTime() - lastSet.getTime()) / (rise.getTime() - lastSet.getTime());
    left = <span>Coucher<b>{absoluteTime(lastSet.toISOString())}</b></span>;
    right = <span>Lever<b>{absoluteTime(rise.toISOString())}</b></span>;
    caption = `Nuit · lever ${relative(rise.toISOString(), now, 'dans')}`;
  }
  p = Math.max(0, Math.min(1, p));
  const ang = Math.PI - p * Math.PI, x = 50 + 45 * Math.cos(ang), y = 50 - 45 * Math.sin(ang);
  return (
    <div className="sun">
      <div className="sun-arc">
        <svg viewBox="0 0 100 56" aria-hidden="true">
          <line className="sun-ground" x1="0" y1="50" x2="100" y2="50" />
          <path className="sun-path" d="M5 50A45 45 0 0 1 95 50" />
          <path className="sun-done" d={`M5 50A45 45 0 0 1 ${x.toFixed(1)} ${y.toFixed(1)}`} />
          <circle className={cx('sun-dot', !isDay && '-moon')} cx={x.toFixed(1)} cy={y.toFixed(1)} r="2.6" />
        </svg>
      </div>
      <div className="sun-times">{left}{right}</div>
      <div className="sun-dur">{caption}</div>
    </div>
  );
}

export function InOut({ inside, outside, cond, night }: { inside: number | undefined; outside: number | undefined; cond?: string; night?: boolean }) {
  const delta = inside != null && outside != null ? outside - inside : undefined;
  return (
    <div className="inout">
      <div className="inout-side"><span className="inout-l"><Icon name="home" />Dedans</span><Val value={inside} unit="°" size="std" /></div>
      <div className={cx('inout-d', delta != null && (delta > 0 ? '-warm' : '-cold'))}><b>{delta == null ? '—' : `${delta > 0 ? '+' : '−'}${Math.abs(delta).toLocaleString('fr-FR', { maximumFractionDigits: 1 })}°`}</b>dehors</div>
      <div className="inout-side"><span className="inout-l">Dehors<WeatherGlyph cond={cond} night={night} size="xs" hidden /></span><Val value={outside} unit="°" size="std" decimals={0} /></div>
    </div>
  );
}

export function Spark({ values, min, max, tone }: { values: number[]; min?: number; max?: number; tone?: 'cool' | 'muted' }) {
  if (values.length < 2) return null;
  const lo = min ?? Math.min(...values), hi = max ?? Math.max(...values);
  const W = 100, H = 30, pad = 4;
  const x = (i: number) => (i / (values.length - 1)) * W, y = (n: number) => H - pad - ((n - lo) / (hi - lo || 1)) * (H - 2 * pad);
  let d = `M${x(0)} ${y(values[0])}`;
  for (let i = 0; i < values.length - 1; i++) {
    const p0 = values[i - 1] ?? values[i], p1 = values[i], p2 = values[i + 1], p3 = values[i + 2] ?? p2;
    d += ` C${x(i) + (x(i + 1) - x(i)) / 3} ${y(p1) + (y(p2) - y(p0)) / 6} ${x(i + 1) - (x(i + 1) - x(i)) / 3} ${y(p2) - (y(p3) - y(p1)) / 6} ${x(i + 1)} ${y(p2)}`;
  }
  return (
    <svg className={cx('spark', tone && `-${tone}`)} viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" aria-hidden="true">
      <path d={`${d} L${W} ${H} L0 ${H} Z`} className="spark-fill" />
      <path d={d} className="spark-line" />
    </svg>
  );
}

/* ---------- Qualité de l'air (indice ATMO 1–6, polluants, pollens) ---------- */

export const POLLUTANT_LABEL: Record<PollutantKey, ReactNode> = {
  pm25: <>PM<sub>2.5</sub></>, pm10: <>PM<sub>10</sub></>, no2: <>NO<sub>2</sub></>, o3: <>O<sub>3</sub></>, so2: <>SO<sub>2</sub></>,
};
export const POLLUTANT_NAME: Record<PollutantKey, string> = { pm25: 'Particules fines PM₂.₅', pm10: 'Particules PM₁₀', no2: 'Dioxyde d’azote', o3: 'Ozone', so2: 'Dioxyde de soufre' };
export const POLLEN_NAME: Record<PollenKey, string> = { grass: 'Graminées', birch: 'Bouleau', alder: 'Aulne', mugwort: 'Armoise', olive: 'Olivier', ragweed: 'Ambroisie' };
const AIR_LABELS = ['', 'Bon', 'Moyen', 'Dégradé', 'Mauvais', 'Très mauvais', 'Extrêmement mauvais'];

/** Index card: big level badge, label, dominant pollutant, six-step scale. */
export function AirIndex({ air, now }: { air: Air | undefined; now: Date }) {
  if (!air || !air.available || !air.index) {
    return (
      <div className="air">
        <div className="air-head"><span className="air-lvl is-none" role="img" aria-label="Indice indisponible">—</span><div><div className="air-t" style={{ color: 'var(--fg-muted)' }}>Indice indisponible</div><div className="air-s">{air ? `Dernière mesure ${relative(air.updatedAt, now, '')}` : 'Qualité de l’air non configurée'}</div></div></div>
        <div className="air-scale" aria-hidden="true">{[1, 2, 3, 4, 5, 6].map((i) => <i key={i} />)}</div>
        <div className="air-ends"><span>Bon</span><span>Extrêmement mauvais</span></div>
      </div>
    );
  }
  const parts: string[] = [];
  if (air.dominant) parts.push(`${POLLUTANT_NAME[air.dominant]} dominant`);
  if (air.place) parts.push(air.place);
  if (air.tomorrow) parts.push(`demain : ${AIR_LABELS[air.tomorrow].toLowerCase()}`);
  return (
    <div className="air">
      <div className="air-head">
        <span className={cx('air-lvl', `lv${air.index}`)} role="img" aria-label={`Indice ${air.index} sur 6`}>{air.index}</span>
        <div><div className="air-t">Air {air.label.toLowerCase()}</div><div className="air-s">{parts.join(' · ')}</div></div>
      </div>
      <div className="air-scale" aria-hidden="true">{[1, 2, 3, 4, 5, 6].map((i) => <i key={i} className={cx(i <= air.index && 'is-on', i === air.index && 'is-now')} />)}</div>
      <div className="air-ends"><span>Bon</span><span>Extrêmement mauvais</span></div>
    </div>
  );
}

/** Pollutants: sub-index bar (1–6) and concentration, dominant first. */
export function AirPollutants({ air }: { air: Air }) {
  const rows = [...air.pollutants].sort((a, b) => (a.key === air.dominant ? -1 : b.key === air.dominant ? 1 : b.index - a.index));
  return (
    <div className="pollu">
      {rows.map((p) => (
        <div key={p.key} className={cx('pollu-row', p.key === air.dominant && 'is-main')}>
          <b>{POLLUTANT_LABEL[p.key]}</b>
          <div className="pollu-bar" aria-hidden="true">{[1, 2, 3, 4, 5, 6].map((i) => <i key={i} className={cx(i <= p.index && 'is-on')} />)}</div>
          <span className="pollu-v">{Math.round(p.value)} <small>µg/m³</small></span>
        </div>
      ))}
    </div>
  );
}

/** Pollens by species, level 0–6 as dots; species at 0 are dimmed. */
export function AirPollens({ air }: { air: Air }) {
  const rows = [...air.pollens].sort((a, b) => b.level - a.level);
  return (
    <div className="pollen">
      {rows.map((p) => (
        <div key={p.key} className={cx('pollen-item', `lv${p.level}`, p.level === 0 && 'is-off')}>
          <b><Icon name="flower" />{POLLEN_NAME[p.key]}</b>
          <span className="dots" role="img" aria-label={`${POLLEN_NAME[p.key]} niveau ${p.level} sur 6`}>{[1, 2, 3, 4, 5, 6].map((i) => <i key={i} className={cx(i <= p.level && 'is-on')} />)}</span>
        </div>
      ))}
    </div>
  );
}

/** Small chip for headers / tiles: "Air moyen". */
export function AirChip({ air }: { air: Air | undefined }) {
  if (!air || !air.available || !air.index) return null;
  return <span className={cx('chip', `is-air-${air.index}`)}><Icon name="air" />Air {air.label.toLowerCase()}</span>;
}
