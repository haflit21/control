// French formatting, through Intl: numbers, temperatures, relative times,
// plurals. Thresholds and wording are ours.

const nf1 = new Intl.NumberFormat('fr-FR', { minimumFractionDigits: 0, maximumFractionDigits: 1 });
const nf0 = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 });
const NBSP = ' '; // narrow no-break space before units
const MINUS = '−';

function sign(s: string) {
  return s.replace('-', MINUS);
}

/** 19,5 (no unit) — 1 decimal for room temperatures. */
export function temp1(v: number | undefined | null): string {
  return v == null ? '—' : sign(nf1.format(v));
}
/** 9 (no unit) — integer for weather. */
export function temp0(v: number | undefined | null): string {
  return v == null ? '—' : sign(nf0.format(v));
}
export function int(v: number | undefined | null): string {
  return v == null ? '—' : sign(nf0.format(v));
}
export function withUnit(v: string, unit: string): string {
  return v === '—' ? v : `${v}${NBSP}${unit}`;
}

/** 1 allumée / 3 allumées */
export function plural(n: number, one: string, many: string): string {
  const pr = new Intl.PluralRules('fr-FR');
  return `${nf0.format(n)}${NBSP}${pr.select(n) === 'one' ? one : many}`;
}

const tf = new Intl.DateTimeFormat('fr-FR', { hour: '2-digit', minute: '2-digit' });
const df = new Intl.DateTimeFormat('fr-FR', { day: '2-digit', month: '2-digit' });

/** "il y a 3 min", "depuis 20 min", or absolute beyond 24 h. */
export function relative(iso: string | undefined, now: Date, prefix: 'il y a' | 'depuis' | 'dans' | '' = 'il y a'): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '—';
  const diff = (now.getTime() - d.getTime()) / 1000;
  const abs = Math.abs(diff);
  if (abs >= 24 * 3600) return `${df.format(d)} à ${tf.format(d)}`;
  let v: string;
  if (abs < 60) v = `${Math.round(abs)}${NBSP}s`;
  else if (abs < 3600) v = `${Math.round(abs / 60)}${NBSP}min`;
  else v = `${Math.round(abs / 3600)}${NBSP}h`;
  if (prefix === '') return v;
  if (prefix === 'dans' || diff < 0) return `dans ${v}`;
  return `${prefix} ${v}`;
}

export function absoluteTime(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  return isNaN(d.getTime()) ? '' : tf.format(d);
}

export function clockHM(d: Date): string {
  return tf.format(d);
}

/** "Bonjour" / "Bonsoir" by the hour. */
export function greeting(now: Date): string {
  const h = now.getHours();
  return h >= 5 && h < 18 ? 'Bonjour' : 'Bonsoir';
}

const longDate = new Intl.DateTimeFormat('fr-FR', { weekday: 'long', day: 'numeric', month: 'long' });
export function dateLong(now: Date): string {
  const s = longDate.format(now);
  return s.charAt(0).toUpperCase() + s.slice(1);
}

const weekday = new Intl.DateTimeFormat('fr-FR', { weekday: 'short' });
export function dayShort(iso: string, now: Date): string {
  const d = new Date(iso);
  if (d.toDateString() === now.toDateString()) return 'Auj.';
  return weekday.format(d);
}
export function hourShort(iso: string, now: Date): string {
  const d = new Date(iso);
  if (Math.abs(d.getTime() - now.getTime()) < 30 * 60 * 1000) return 'Maint.';
  return `${d.getHours()}${NBSP}h`;
}

export const CONDITION_LABEL: Record<string, string> = {
  sunny: 'Ensoleillé', 'clear-night': 'Nuit claire', partlycloudy: 'Éclaircies', cloudy: 'Couvert', fog: 'Brouillard',
  rainy: 'Pluie', pouring: 'Fortes pluies', lightning: 'Orages', 'lightning-rainy': 'Orages et pluie', snowy: 'Neige',
  'snowy-rainy': 'Neige et pluie', hail: 'Grêle', windy: 'Venteux', 'windy-variant': 'Vent et nuages', exceptional: 'Phénomène exceptionnel',
};
export const CONDITION_LABEL_NIGHT: Record<string, string> = { partlycloudy: 'Nuit nuageuse', sunny: 'Nuit claire' };

export function conditionLabel(cond: string | undefined, night = false): string {
  if (!cond) return '—';
  return (night && CONDITION_LABEL_NIGHT[cond]) || CONDITION_LABEL[cond] || cond;
}

export const CARDINALS = ['N', 'NE', 'E', 'SE', 'S', 'SO', 'O', 'NO'];
export const CARDINALS_LONG = ['Nord', 'Nord-est', 'Est', 'Sud-est', 'Sud', 'Sud-ouest', 'Ouest', 'Nord-ouest'];
export function cardinal(deg: number | undefined, long = false): string {
  if (deg == null) return '—';
  const i = Math.round(((deg % 360) + 360) % 360 / 45) % 8;
  return (long ? CARDINALS_LONG : CARDINALS)[i];
}

export function uvLevel(uv: number | undefined): { lv: 1 | 2 | 3 | 4 | 5; label: string } | undefined {
  if (uv == null) return undefined;
  if (uv < 3) return { lv: 1, label: 'faible' };
  if (uv < 6) return { lv: 2, label: 'modéré' };
  if (uv < 8) return { lv: 3, label: 'élevé' };
  if (uv < 11) return { lv: 4, label: 'très élevé' };
  return { lv: 5, label: 'extrême' };
}

export const HEATING_MODE_LABEL: Record<string, string> = { schedule: 'Planning', manual: 'Manuel', off: 'Arrêt', mixed: 'Mixte', night: 'Nuit', boost: 'Boost' };
export const AC_MODE_LABEL: Record<string, string> = { cool: 'Froid', heat: 'Chaud', fan: 'Ventilation', auto: 'Auto', dry: 'Déshumidification', off: 'Arrêt' };

export { NBSP, MINUS };
