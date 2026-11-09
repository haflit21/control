// Data layer: one query for /api/home, kept fresh by the SSE stream.
import { useEffect, useState } from 'react';
import { QueryClient, useQuery, useQueryClient } from '@tanstack/react-query';
import { applyEvent, type Home, type Event, type ApiError, type JournalEntry } from '@maison/model';
import { confirmPending } from './actions';

export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 60_000, refetchOnWindowFocus: true, retry: 1 } },
});

export class HttpError extends Error {
  constructor(public status: number, public body: ApiError) {
    super(body.error);
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { ...init, headers: { 'X-Maison': '1', 'Content-Type': 'application/json', ...(init?.headers ?? {}) } });
  if (!res.ok) {
    let body: ApiError = { error: `HTTP ${res.status}` };
    try { body = await res.json(); } catch { /* not JSON */ }
    throw new HttpError(res.status, body);
  }
  return res.json() as Promise<T>;
}

export const HOME_KEY = ['home'] as const;

export function useHome() {
  return useQuery<Home, HttpError>({ queryKey: HOME_KEY, queryFn: () => api<Home>('/api/home') });
}

export const JOURNAL_KEY = ['journal'] as const;

/** Last actions (who, what, when), newest first. Refreshed after each action and every minute. */
export function useJournal(limit = 30) {
  return useQuery<{ entries: JournalEntry[]; enabled: boolean }, HttpError>({ queryKey: [...JOURNAL_KEY, limit], queryFn: () => api(`/api/journal?limit=${limit}`), refetchInterval: 60_000, staleTime: 10_000 });
}

export type Link = { state: 'connecting' | 'open' | 'lost'; since: number };

/** Opens the SSE stream once and applies events to the home query. */
export function useEvents(): Link {
  const qc = useQueryClient();
  const [link, setLink] = useState<Link>({ state: 'connecting', since: Date.now() });
  useEffect(() => {
    let es: EventSource | undefined;
    let stopped = false;
    let retry = 1000;
    const open = () => {
      if (stopped) return;
      es = new EventSource('/api/events');
      es.onopen = () => { retry = 1000; setLink({ state: 'open', since: Date.now() }); };
      const on = (type: Event['type']) => (m: MessageEvent) => {
        try {
          const ev = JSON.parse(m.data) as Event;
          if (ev.type !== type) return;
          if (ev.type === 'home') qc.setQueryData<Home>(HOME_KEY, ev.home);
          else qc.setQueryData<Home>(HOME_KEY, (h) => (h ? applyEvent(h, ev) : h));
          if (ev.type === 'device') confirmPending(ev.device.id);
        } catch { /* ignore a malformed event */ }
      };
      es.addEventListener('home', on('home'));
      es.addEventListener('device', on('device'));
      es.addEventListener('summary', on('summary'));
      es.addEventListener('status', on('status'));
      es.onerror = () => {
        es?.close();
        setLink((l) => (l.state === 'lost' ? l : { state: 'lost', since: Date.now() }));
        if (!stopped) setTimeout(open, retry);
        retry = Math.min(retry * 2, 15_000);
      };
    };
    open();
    return () => { stopped = true; es?.close(); };
  }, [qc]);
  return link;
}

/** A clock that ticks every 30 s, for relative times and staleness. */
export function useNow(intervalMs = 30_000): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), intervalMs);
    return () => clearInterval(t);
  }, [intervalMs]);
  return now;
}

/** Values are greyed after 2 min without news from HA or from the server. */
export function isStale(home: Home | undefined, link: Link, now: Date): boolean {
  if (!home) return false;
  if (!home.connected) return true;
  return link.state !== 'open' && now.getTime() - link.since > 2 * 60_000;
}

/** Camera snapshot URL, refreshed by a changing key. */
export function snapshotURL(deviceId: string, tick: number): string {
  return `/api/cameras/${encodeURIComponent(deviceId)}/snapshot?t=${tick}`;
}

/** A counter that increments every n seconds (snapshot refresh). */
export function useTick(seconds: number): number {
  const [t, setT] = useState(0);
  useEffect(() => {
    const i = setInterval(() => setT((x) => x + 1), seconds * 1000);
    return () => clearInterval(i);
  }, [seconds]);
  return t;
}

/** True when the media query matches (updates live). */
export function useMedia(query: string): boolean {
  const [m, setM] = useState(() => (typeof window !== 'undefined' ? window.matchMedia(query).matches : false));
  useEffect(() => {
    const mq = window.matchMedia(query);
    const on = () => setM(mq.matches);
    on();
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  }, [query]);
  return m;
}
