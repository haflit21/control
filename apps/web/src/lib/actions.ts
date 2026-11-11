// Actions (étape 2): POST to the server, optimistic update of the home query,
// pending state until HA confirms, rollback + toast when it does not.
import { useCallback, useSyncExternalStore } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Device, DeviceAction, Home, State } from '@control/model';
import { HOME_KEY, JOURNAL_KEY, HttpError, api } from './api';

/* ---------- toasts ---------- */
export type Toast = { id: number; title: string; detail?: string; tone?: 'ok' | 'accent' | 'error'; action?: { label: string; run: () => void }; until?: number };
let toasts: Toast[] = [];
const listeners = new Set<() => void>();
let seq = 1;
function emit() { listeners.forEach((l) => l()); }
export function pushToast(t: Omit<Toast, 'id'>): number {
  const id = seq++;
  const ttl = t.tone === 'error' ? 8000 : 3500;
  toasts = [...toasts.filter((x) => x.title !== t.title || x.detail !== t.detail), { ...t, id, until: Date.now() + ttl }];
  emit();
  setTimeout(() => dismissToast(id), ttl);
  return id;
}
export function dismissToast(id: number) {
  if (!toasts.some((t) => t.id === id)) return;
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}
export function useToasts(): Toast[] {
  return useSyncExternalStore((l) => { listeners.add(l); return () => listeners.delete(l); }, () => toasts, () => toasts);
}

/* ---------- pending (device id → until) ---------- */
let pending: Record<string, number> = {};
const pendingListeners = new Set<() => void>();
function setPending(id: string, on: boolean) {
  if (on) pending = { ...pending, [id]: Date.now() + 8000 };
  else { const { [id]: _drop, ...rest } = pending; pending = rest; }
  pendingListeners.forEach((l) => l());
}
export function usePending(): Record<string, number> {
  return useSyncExternalStore((l) => { pendingListeners.add(l); return () => pendingListeners.delete(l); }, () => pending, () => pending);
}

/* ---------- optimistic predictions ---------- */
/** What the device state will look like once HA applies the action. */
export function predict(d: Device, a: DeviceAction): Partial<State> {
  switch (a.action) {
    case 'power': return d.kind === 'ac' ? { on: a.params.on, mode: a.params.on ? 'cool' : 'off' } : { on: a.params.on };
    case 'set_brightness': return a.params.pct <= 0 ? { on: false } : { on: true, brightness: a.params.pct };
    case 'set_color_temp': return { on: true, colorTemp: a.params.kelvin, color: undefined };
    case 'set_color': return { on: true, color: a.params.hs };
    case 'open': return { position: 100, moving: 'opening' as State['moving'] };
    case 'close': return { position: 0, moving: 'closing' as State['moving'] };
    case 'stop': return { moving: undefined };
    case 'set_position': return { position: a.params.pct };
    case 'set_target': return { target: a.params.temperature, heating: d.state.temperature != null ? a.params.temperature > d.state.temperature : d.state.heating, mode: d.kind === 'heating' ? 'manual' : d.state.mode };
    case 'set_mode': return d.kind === 'heating' ? { mode: a.params.mode, heating: a.params.mode === 'off' ? false : d.state.heating } : { mode: a.params.mode as State['mode'], on: a.params.mode !== 'off' };
    case 'play': return { playing: 'playing', elapsed: elapsedNow(d.state), elapsedAt: new Date().toISOString() };
    case 'pause': return { playing: 'paused', elapsed: elapsedNow(d.state), elapsedAt: new Date().toISOString() };
    case 'next': case 'previous': return d.state.duration != null ? { elapsed: 0, elapsedAt: new Date().toISOString() } : {};
    case 'seek': return { elapsed: a.params.position, elapsedAt: new Date().toISOString() };
    case 'set_volume': return { volume: a.params.pct };
    case 'play_radio': return { playing: 'playing', radio: a.params.id };
    case 'play_music': return a.params.kind === 'album'
      ? { playing: 'playing', radio: undefined, album: a.params.title ?? d.state.album, artist: a.params.artists ?? d.state.artist, elapsed: 0, elapsedAt: new Date().toISOString() }
      : { playing: 'playing', radio: undefined, uri: a.params.uri, title: a.params.title ?? d.state.title, artist: a.params.artists ?? d.state.artist, elapsed: 0, elapsedAt: new Date().toISOString() };
    case 'set_privacy': return { privacy: a.params.on };
    case 'set_eco': return { eco: a.params.on };
    case 'set_boost': return { boost: a.params.on };
  }
  return {};
}

/** Seconds played right now: the last known position, plus the time since if it plays. */
export function elapsedNow(s: State, now = Date.now()): number | undefined {
  if (s.elapsed == null) return undefined;
  if (s.playing !== 'playing' || !s.elapsedAt) return s.elapsed;
  const at = Date.parse(s.elapsedAt);
  const v = Number.isNaN(at) ? s.elapsed : s.elapsed + Math.max(0, (now - at) / 1000);
  return s.duration != null ? Math.min(v, s.duration) : v;
}

function patchDevice(h: Home, id: string, patch: Partial<State>): Home {
  return { ...h, rooms: h.rooms.map((r) => ({ ...r, devices: r.devices.map((d) => (d.id === id ? { ...d, state: { ...d.state, ...patch } } : d)) })) };
}

export function useActions() {
  const qc = useQueryClient();

  const device = useCallback(async (d: Device, a: DeviceAction, opts?: { quiet?: boolean }) => {
    const before = qc.getQueryData<Home>(HOME_KEY);
    const patch = predict(d, a);
    if (before) qc.setQueryData<Home>(HOME_KEY, patchDevice(before, d.id, patch));
    setPending(d.id, true);
    try {
      const res = await api<{ ok: boolean; message?: string }>(`/api/devices/${encodeURIComponent(d.id)}/actions`, { method: 'POST', body: JSON.stringify(a) });
      if (!opts?.quiet && res.message) pushToast({ title: res.message, tone: 'ok' });
      void qc.invalidateQueries({ queryKey: JOURNAL_KEY });
      // the SSE confirms; if nothing comes back in 8 s, revert
      setTimeout(() => { if (pending[d.id] && pending[d.id] <= Date.now()) { setPending(d.id, false); } }, 8200);
    } catch (e) {
      setPending(d.id, false);
      if (before) qc.setQueryData<Home>(HOME_KEY, (h) => (h ? patchDevice(h, d.id, before.rooms.flatMap((r) => r.devices).find((x) => x.id === d.id)?.state ?? {}) : h));
      const err = e instanceof HttpError ? e : undefined;
      pushToast({ title: `${d.name} : ${err?.body.error ?? 'pas de réponse'}`, detail: err?.body.detail, tone: 'error' });
      throw e;
    }
  }, [qc]);

  const house = useCallback(async (path: string, body: unknown, optimistic?: (h: Home) => Home) => {
    const before = qc.getQueryData<Home>(HOME_KEY);
    if (before && optimistic) qc.setQueryData<Home>(HOME_KEY, optimistic(before));
    try {
      const res = await api<{ ok: boolean; message?: string }>(path, { method: 'POST', body: JSON.stringify(body ?? {}) });
      if (res.message) pushToast({ title: res.message, tone: 'ok' });
      void qc.invalidateQueries({ queryKey: JOURNAL_KEY });
    } catch (e) {
      if (before) qc.setQueryData<Home>(HOME_KEY, before);
      const err = e instanceof HttpError ? e : undefined;
      pushToast({ title: err?.body.error ?? 'Pas de réponse du serveur', detail: err?.body.detail, tone: 'error' });
      throw e;
    }
  }, [qc]);

  return { device, house };
}

/** Clears the pending mark of a device once a fresh state arrived from the server. */
export function confirmPending(id: string) {
  if (pending[id]) setPending(id, false);
}
